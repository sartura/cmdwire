use cmdwire::{Field, Kind, parse, parse_bytes};
use serde::Deserialize;

#[derive(Deserialize)]
struct Corpus {
    version: u64,
    valid: Vec<ValidCase>,
    invalid: Vec<InvalidCase>,
}

#[derive(Deserialize)]
struct ValidCase {
    line: String,
    record: ExpectedRecord,
}

#[derive(Deserialize)]
struct InvalidCase {
    line: String,
}

#[derive(Deserialize)]
struct ExpectedRecord {
    kind: String,
    command: String,
    #[serde(default)]
    resource: Option<String>,
    #[serde(default)]
    fields: Vec<ExpectedField>,
}

#[derive(Deserialize)]
struct ExpectedField {
    name: String,
    value: String,
}

#[test]
fn shared_parser_corpus() {
    let corpus: Corpus = serde_json::from_str(include_str!("../../../testdata/conformance.json"))
        .expect("decode conformance corpus");
    assert_eq!(corpus.version, 1);

    for case in corpus.valid {
        let record = parse(&case.line)
            .unwrap_or_else(|error| panic!("valid line {:?} was rejected: {error}", case.line));
        let byte_record = parse_bytes(case.line.as_bytes()).unwrap_or_else(|error| {
            panic!("valid byte line {:?} was rejected: {error}", case.line)
        });
        assert_eq!(byte_record, record, "line {:?}", case.line);
        assert_eq!(
            record.kind(),
            parse_kind(&case.record.kind),
            "line {:?}",
            case.line
        );
        assert_eq!(
            record.command(),
            case.record.command,
            "line {:?}",
            case.line
        );
        assert_eq!(
            record.resource(),
            case.record.resource.as_deref(),
            "line {:?}",
            case.line
        );
        let fields: Vec<Field<'_>> = record.fields().collect();
        let expected: Vec<Field<'_>> = case
            .record
            .fields
            .iter()
            .map(|field| Field {
                name: &field.name,
                value: &field.value,
            })
            .collect();
        assert_eq!(fields, expected, "line {:?}", case.line);
    }

    for case in corpus.invalid {
        assert!(
            parse(&case.line).is_err(),
            "invalid line accepted: {:?}",
            case.line
        );
        assert!(
            parse_bytes(case.line.as_bytes()).is_err(),
            "invalid byte line accepted: {:?}",
            case.line
        );
    }
}

fn parse_kind(kind: &str) -> Kind {
    match kind {
        "request" => Kind::Request,
        "ok" => Kind::Ok,
        "err" => Kind::Error,
        "notice" => Kind::Notice,
        "event" => Kind::Event,
        "item" => Kind::Item,
        "chunk" => Kind::Chunk,
        _ => panic!("unknown corpus kind {kind:?}"),
    }
}
