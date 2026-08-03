use std::fmt::Write as _;
use std::io::{self, BufRead};

fn main() -> io::Result<()> {
    for line in io::stdin().lock().lines() {
        let input = decode_hex(&line?).map_err(io::Error::other)?;
        println!("{}", describe(&input));
    }
    Ok(())
}

fn decode_hex(input: &str) -> Result<Vec<u8>, &'static str> {
    if !input.len().is_multiple_of(2) {
        return Err("odd hexadecimal input length");
    }
    input
        .as_bytes()
        .chunks_exact(2)
        .map(|pair| {
            let high = digit(pair[0]).ok_or("invalid hexadecimal input")?;
            let low = digit(pair[1]).ok_or("invalid hexadecimal input")?;
            Ok((high << 4) | low)
        })
        .collect()
}

fn digit(value: u8) -> Option<u8> {
    match value {
        b'0'..=b'9' => Some(value - b'0'),
        b'a'..=b'f' => Some(value - b'a' + 10),
        _ => None,
    }
}

fn describe(input: &[u8]) -> String {
    let Ok(record) = cmdwire::parse_line_bytes(input) else {
        return "reject".to_owned();
    };
    let mut output = format!(
        "accept\t{}\t{}\t{}",
        record.kind().as_str(),
        record.command(),
        record.resource().unwrap_or("")
    );
    for field in record.fields() {
        write!(output, "\t{}={}", field.name, field.value).expect("writing to String cannot fail");
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn describes_acceptance_and_rejection() {
        assert_eq!(
            describe(b"item object.status Alpha state=ready"),
            "accept\titem\tobject.status\tAlpha\tstate=ready"
        );
        assert_eq!(
            describe(b"item object.status Alpha state=ready\n"),
            "accept\titem\tobject.status\tAlpha\tstate=ready"
        );
        assert_eq!(
            describe(b"item object.status Alpha state=ready\r\n"),
            "accept\titem\tobject.status\tAlpha\tstate=ready"
        );
        assert_eq!(describe(b"not a record"), "reject");
    }

    #[test]
    fn decodes_transport_hexadecimal() {
        assert_eq!(decode_hex("00ff"), Ok(vec![0, 255]));
        assert!(decode_hex("0").is_err());
        assert!(decode_hex("gg").is_err());
    }
}
