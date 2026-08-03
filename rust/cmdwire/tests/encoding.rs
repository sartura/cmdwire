use cmdwire::{
    EncodeError, Kind, ParseErrorKind, WireField, encode_record, parse, parse_bytes, parse_line,
    parse_line_bytes,
};

#[test]
fn encodes_bounded_records_without_allocation() {
    let line = encode_record(
        Kind::Item,
        "object.status",
        Some("alpha"),
        [
            Some(WireField::token("state", "ready")),
            Some(WireField::boolean("available", true)),
            Some(WireField::uint("count", 7)),
            Some(WireField::hex("counter", 0x2a, 8)),
        ],
    )
    .expect("encode record");
    assert_eq!(
        line.as_str(),
        "item object.status alpha state=ready available=true count=7 counter=0x0000002a"
    );
    parse(line.as_str()).expect("encoded record parses");
}

#[test]
fn encodes_notice_records() {
    let line = encode_record(
        Kind::Notice,
        "system.boot",
        Some("storage"),
        [Some(WireField::token("state", "ready"))],
    )
    .expect("encode notice");
    assert_eq!(line.as_str(), "notice system.boot storage state=ready");
    assert_eq!(
        parse(line.as_str()).expect("parse notice").kind(),
        Kind::Notice
    );
}

#[test]
fn rejects_empty_notice() {
    let error = encode_record::<0>(Kind::Notice, "system.boot", None, [])
        .expect_err("empty notice accepted");
    assert_eq!(error, EncodeError::InvalidRecord);
}

#[test]
fn parses_optional_line_endings() {
    let body = "request object.status";
    let want = parse(body).expect("parse record body");

    assert_eq!(parse_line(body), Ok(want));
    assert_eq!(parse_line("request object.status\n"), Ok(want));
    assert_eq!(parse_line("request object.status\r\n"), Ok(want));
    assert_eq!(parse_line_bytes(b"request object.status\n"), Ok(want));
    assert_eq!(parse_line_bytes(b"request object.status\r\n"), Ok(want));

    assert!(parse_line("request object.status\n\n").is_err());
    assert!(parse_line("request\nobject.status").is_err());
    assert!(parse_line("request object.status\r").is_err());
}

#[test]
fn validates_every_byte_before_unchecked_utf8_conversion() {
    for byte in u8::MIN..=u8::MAX {
        let input = [byte];
        let result = parse_bytes(&input);
        if !(0x20..=0x7e).contains(&byte) {
            let error = result.expect_err("non-ASCII byte accepted");
            assert_eq!(error.column, 1, "byte {byte:#04x}");
            assert_eq!(error.kind, ParseErrorKind::NonAscii, "byte {byte:#04x}");
        }
    }
}

#[test]
fn rejects_non_ascii_bytes() {
    let error =
        parse_bytes(b"request object.set value=\xff").expect_err("non-ASCII input accepted");
    assert_eq!(error.kind, ParseErrorKind::NonAscii);
}

#[test]
fn rejects_oversized_records_before_transport() {
    let error = encode_record(
        Kind::Request,
        "object.set",
        None,
        [Some(WireField::token(
            "value",
            "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        ))],
    )
    .expect_err("oversized record accepted");
    assert_eq!(error, EncodeError::TooLong);
}

#[test]
fn rejects_invalid_success_terminal() {
    let error = encode_record(
        Kind::Ok,
        "object.status",
        None,
        [
            Some(WireField::uint("schema", 0)),
            Some(WireField::uint("count", 0)),
        ],
    )
    .expect_err("zero schema accepted");
    assert_eq!(error, EncodeError::InvalidRecord);
}

#[test]
fn rejects_invalid_error_code() {
    let error = encode_record(
        Kind::Error,
        "object.set",
        None,
        [Some(WireField::token("code", "bad"))],
    )
    .expect_err("invalid error code accepted");
    assert_eq!(error, EncodeError::InvalidRecord);
}
