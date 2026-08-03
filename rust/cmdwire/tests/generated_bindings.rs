use cmdwire::{Available, parse};

mod generated;

use generated::{
    COMMAND_NAMES, DecodedCommand, ObjectActionError, ObjectActionReply, ObjectListError,
    ObjectListReply, ObjectListReplyEntry, ObjectListReplyTerminal, ObjectStatusReply,
    decode_command, decode_object_action, decode_object_list, decode_object_status,
};

#[test]
fn decodes_typed_request() {
    let record =
        parse("request object.action alpha enabled=true limit=25 mode=safe address=0x0000002a")
            .expect("parse request");
    let request = decode_object_action(record).expect("decode request");
    assert!(request.enabled);
    assert_eq!(request.limit, Some(25));
    assert_eq!(request.mode, Some("safe"));
    assert_eq!(request.address, Some(0x2a));
}

#[test]
fn dispatches_typed_commands() {
    assert_eq!(COMMAND_NAMES.len(), 3);
    let record = parse("request object.status").expect("parse status");
    assert!(matches!(
        decode_command(record).expect("dispatch status"),
        DecodedCommand::ObjectStatus(_)
    ));
    let unknown = parse("request object.unknown").expect("parse unknown");
    assert_eq!(
        decode_command(unknown),
        Err(cmdwire::DecodeError::UnknownCommand)
    );
}

#[test]
fn encodes_typed_reply() {
    let lines = ObjectActionReply {
        state_state: "ready",
        state_counter: 0x2a,
        telemetry_reading: Available::Unavailable,
        telemetry_note: Some("stable"),
        terminal_changed: true,
    }
    .encode()
    .expect("encode reply");

    assert_eq!(
        lines.map(|line| line.as_str().to_owned()),
        [
            "item object.action state state=ready counter=0x0000002a",
            "item object.action telemetry reading=unavailable note=stable",
            "ok object.action schema=1 count=2 changed=true",
        ]
    );
}

#[test]
fn streams_bounded_variable_reply_records() {
    let request =
        decode_object_list(parse("request object.list").expect("parse list")).expect("decode list");
    assert_eq!(request, generated::ObjectListRequest {});

    let mut reply = ObjectListReply::new();
    let first = reply
        .push_entry(ObjectListReplyEntry {
            name: "alpha",
            value: Some(1),
            data: Some(&[0xde, 0xad]),
        })
        .expect("encode first item");
    let second = reply
        .push_entry(ObjectListReplyEntry {
            name: "beta",
            value: None,
            data: None,
        })
        .expect("encode second item");
    let terminal = reply
        .finish(ObjectListReplyTerminal { complete: true })
        .expect("encode terminal");

    assert_eq!(
        first.as_str(),
        "item object.list entry name=alpha value=1 data=dead"
    );
    assert_eq!(second.as_str(), "item object.list entry name=beta");
    assert_eq!(
        terminal.as_str(),
        "ok object.list schema=1 count=2 complete=true"
    );
    assert_eq!(
        ObjectListReply::new().finish(ObjectListReplyTerminal { complete: true }),
        Err(cmdwire::EncodeError::InvalidReplyState)
    );
    assert_eq!(
        ObjectListError::BadRequest
            .encode()
            .expect("encode list error")
            .as_str(),
        "err object.list code=BAD_REQUEST"
    );
}

#[test]
fn supports_empty_request_and_reply_shapes() {
    let request = decode_object_status(parse("request object.status").expect("parse status"))
        .expect("decode status");
    assert_eq!(request, generated::ObjectStatusRequest {});

    let [terminal] = ObjectStatusReply {}.encode().expect("encode status");
    assert_eq!(terminal.as_str(), "ok object.status schema=1 count=0");
}

#[test]
fn encodes_declared_error() {
    let line = ObjectActionError::BadRequest {
        field: Some("limit"),
    }
    .encode()
    .expect("encode error");
    assert_eq!(
        line.as_str(),
        "err object.action code=BAD_REQUEST field=limit"
    );

    let busy = ObjectActionError::Busy.encode().expect("encode busy error");
    assert_eq!(busy.as_str(), "err object.action code=BUSY");
}
