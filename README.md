# cmdwire

cmdwire is a bounded, line-oriented request, notice, and reply protocol for
command consoles. It uses one readable wire format across serial consoles,
automated qualification, remote command adapters, and other ordered text
transports.

The protocol, Go implementation, and Rust implementation are drafts. Expect
incompatible changes before the first stable release.

## Contents

- [Quick start](#quick-start)
- [Scope and security](#scope-and-security)
- [Protocol specification](#cmdwire-protocol)
- [Implementation architecture](#implementation-architecture)
- [Verification](#verification)
- [Contributing](#contributing)

## Quick start

### Wire example

```text
notice system.boot storage state=ready
request object.observe count=2 timeout=30
event object.observe phase=collecting
event object.observe alpha/telemetry phase=sampled
ok object.observe schema=1 count=2 stop=count
```

A client starts a command with `request`.

See [cmdwire Protocol](#cmdwire-protocol) below for the normative protocol.

### Go package

```go
record, err := cmdwire.ParseLine(
    `item object.status alpha state=ready`,
)
if err != nil {
    return err
}

line, err := cmdwire.Format(record)
```

`ParseLine` enforces the grammar, token-only ASCII values, exact spacing,
duplicate-field rejection, and the 80-byte limit. `Format` emits canonical
ASCII and refuses an oversized result.

Use a `Collector` to extract one reply from interleaved console output:

```go
collector, err := cmdwire.NewCollector("object.status")
if err != nil {
    return err
}
for _, line := range consoleLines {
    _, complete, err := collector.AddLine(line)
    if err != nil {
        return err
    }
    if complete {
        break
    }
}
report, err := collector.Result()
```

`Collector.Result` returns a terminal `err` as `*cmdwire.RemoteError` and
discards earlier data records. `NewCollector` buffers at most 1,024 data
records; use `NewCollectorWithLimit` to choose a smaller application limit.

### Command-line validator

Install the `cmdwire` executable:

```sh
go install github.com/sartura/cmdwire/cmd/cmdwire@latest
```

Validate protocol records from stdin or files:

```sh
cmdwire check transcript.txt
```

Canonicalize valid records:

```sh
cmdwire format transcript.txt
```

`check` is quiet on success. Both commands report the first invalid file, line,
and byte column.

### Rust crate

The Rust crate under `rust/cmdwire` provides allocation-free parsing and
encoding for `no_std` firmware. It targets Rust 1.97.1 and compiles for
`aarch64-unknown-none`.

```rust
let record = cmdwire::parse_line("request object.status\r\n")?;
assert_eq!(record.command(), "object.status");
```

`parse_line` and `parse_line_bytes` accept a record body with an optional LF or
CRLF ending. Use `parse` or `parse_bytes` when the transport has already removed
the line ending. `Kind` is non-exhaustive, so downstream matches must include a
wildcard arm. `Line` stores an encoded record in a fixed 80-byte buffer, so an
oversized record fails before reaching the transport.

## Scope and security

cmdwire defines framing, lexical rules, generic observation limits, and the
ordered record model. Command schemas define resources, fields, types, units,
ordering, identity, and completeness. Parsing never authorizes or executes a
command.

cmdwire does not authenticate records or protect their integrity. Use a trusted
or authenticated transport across security boundaries.

---

# cmdwire Protocol

cmdwire is a line-oriented request, notice, and reply protocol for command
consoles. It combines readable wire records with deterministic parsing, bounded
records, unsolicited notices, command-scoped events, and explicit completion.

## 1. Conformance language

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHOULD**, **SHOULD NOT**,
and **MAY** are to be interpreted as described by RFC 2119 and RFC 8174.

Implementations conform by following the lexical and lifecycle rules in this
specification. Command schemas add resources, fields, types, units, ordering,
and completeness rules.

## 2. Transport

cmdwire runs over an ordered, reliable byte stream. A physical record:

- occupies one line;
- contains printable ASCII bytes `0x20` through `0x7e` only;
- is at most 80 bytes, excluding its line ending;
- uses one ASCII space between tokens;
- has no leading or trailing space;
- ends in LF or CRLF when carried in a stream.

A sender MUST NOT wrap or truncate a record. A schema MUST divide a wide object
into semantic resources or bounded chunks.

Clients send `request` records. Servers send `notice`, `event`, `item`, `chunk`,
`ok`, and `err` records. Other console output may share the transport.
A producer MUST prefix non-protocol output that would otherwise parse as a
cmdwire record.

## 3. Requests and notices

### 3.1 Request

`request` starts a command and carries its name, optional resource path, and
ordered fields.

```text
request object.status
request object.action alpha mode=fast
request object.observe count=2 timeout=30
```

A client MUST NOT start a second request with the same command name while the
first remains active. A server rejects a request it cannot start with `err`,
normally using `BUSY`, `BAD_REQUEST`, or `UNSUPPORTED`.

### 3.2 Notice

`notice` carries unsolicited server state or a lifecycle transition. It uses a
command-shaped topic, an optional resource path, and at least one field.

```text
notice system.boot storage state=initializing
notice system.boot storage state=ready
```

A notice does not start or terminate a reply, requires no response, and never
contributes to a reply's `count`. A server MAY emit notices between reply
records. A reply collector MUST ignore notices, including a notice whose topic
matches the collected command. The transport preserves notice order but does
not provide retention or replay; consumers that attach later may miss earlier
notices.

## 4. Reply records

A reply contains zero or more data records followed by exactly one terminal
record. Data records are `event`, `item`, and `chunk`. Terminal records are `ok`
and `err`. Replies do not nest.

### 4.1 Successful terminal

`ok` terminates every successful reply. Its first field is a positive `schema`;
its second field is a nonnegative `count`. Additional result fields may follow.
A reply without data records uses `count=0`.

```text
ok object.action schema=1 count=0 state=ready
```

`count` equals the number of matching `event`, `item`, and `chunk` records
before the terminal. It excludes requests, unrelated lines, and the terminal.

### 4.2 Error

`err` terminates a failed reply. Its first field is `code`. Structured
diagnostic fields may follow.

```text
err object.action code=BAD_VALUE field=mode
```

An error invalidates earlier data records for that reply. Codes are stable
uppercase API identifiers.

### 4.3 Event

`event` reports progress or an observation without terminating the reply. It has
an optional resource path and at least one field.

```text
event object.observe state=waiting
event object.observe alpha/telemetry phase=sampled
```

Events are command-scoped. Unsolicited background events are outside the
protocol.

### 4.4 Item

`item` reports one object or semantic facet. It has an optional resource path
and zero or more fields.

```text
item object.status alpha state=ready mode=automatic
```

### 4.5 Chunk

`chunk` carries bounded bulk data. It has an optional resource path and at least
one field.

```text
chunk object.read alpha/data offset=0 data=00112233
```

## 5. Observation

Commands may support observation. Consumers MUST parse `event` records even if
they never request observation.

An observation request carries one or both of these fields:

- `count=N`: stop after `N` matching events;
- `timeout=N`: stop after `N` seconds.

Each value is a positive unsigned decimal integer. When both fields are present,
the first limit reached ends observation. An observation without either field
is invalid.

A successful observation terminal includes `stop=count` or `stop=timeout`.

```text
request object.observe count=2 timeout=30
event object.observe phase=collecting
event object.observe alpha phase=sampled
ok object.observe schema=1 count=2 stop=count
```

The server reports source closure or any failure to observe or stop with `err`.

## 6. Normative wire grammar

The following normative ABNF uses RFC 5234 notation. `SP` is one ASCII space.

```abnf
record          = request / notice / success / error / event / item / chunk

request         = "request" SP command [SP resource] *(SP field)
notice          = "notice" SP command [SP resource] 1*(SP field)
success         = "ok" SP command SP schema-field SP count-field
                  *(SP field)
error           = "err" SP command SP code-field *(SP field)
event           = "event" SP command [SP resource] 1*(SP field)
item            = "item" SP command [SP resource] *(SP field)
chunk           = "chunk" SP command [SP resource] 1*(SP field)

schema-field    = "schema=" positive-decimal
count-field     = "count=" unsigned-decimal
code-field      = "code=" error-code
field           = key "=" value

command         = command-segment *("." command-segment)
command-segment = LCALPHA *(LCALPHA / DIGIT / "_" / "-")
resource        = resource-segment *("/" resource-segment)
resource-segment = resource-part ["." resource-part]
resource-part   = ALNUM / (ALNUM *resource-char ALNUM)
resource-char   = ALNUM / "_" / ":" / "-"
key             = LCALPHA *(LCALPHA / DIGIT / "_")
error-code      = UCALPHA *(UCALPHA / DIGIT / "_")

value           = 1*value-char
value-char      = %x21 / %x23-5B / %x5D-7E
positive-decimal = %x31-39 *DIGIT
unsigned-decimal = "0" / (%x31-39 *DIGIT)

ALNUM           = ALPHA / DIGIT
LCALPHA         = %x61-7A
UCALPHA         = %x41-5A
```

Values are nonempty printable ASCII tokens. Space, double quote, and backslash
are forbidden. The protocol has no quoting or escaping. A command schema uses a
textual encoding such as hexadecimal for arbitrary text or bytes.

## 7. Semantic rules

- A record MUST contain one known kind and one valid command.
- A record MUST NOT repeat a field key.
- Field order is deterministic and schema-defined where it matters.
- Wire values are nonempty ASCII strings. A command schema defines their types
  and units.
- Every `ok` begins with `schema` and `count`; those keys are reserved on `ok`.
- Every `err` begins with `code`; that key is reserved on `err`.
- Only `request`, `notice`, `event`, `item`, and `chunk` may carry a resource
  path.
- A resource path is opaque unless its command schema defines segment meaning.
- Each resource part MUST begin and end with an ASCII letter or digit.
- A parser MUST reject malformed records for the command it is observing.
- A parser MAY ignore unrelated lines and valid records for other commands.
- A command schema decides whether resource paths are forbidden, optional, or
  required for each applicable record kind.

## 8. Command schemas

A command schema defines:

- request resources and fields;
- required and optional reply resources and fields;
- field types, units, and allowed values;
- deterministic field and record ordering where order matters;
- terminal result fields;
- valid empty replies;
- item identity and uniqueness;
- chunk offsets and completeness;
- observation support and stop conditions;
- supported schema versions.

Consumers MUST reject unsupported schema versions, mismatched counts, duplicate
identities, missing fields, invalid values, and incomplete chunks as defined by
the command schema.

cmdwire schema documents provide a machine-readable subset for fixed request
and reply shapes. Format version 1 is JSON with `format`, `command`, `version`,
`request`, `reply`, and `errors` members. Each error declares its stable `code`
and ordered diagnostic fields. Request and terminal fields are ordered. Reply
`records` are ordered and identify their kind, resource, and fields. Field
types are `token`, `bool`, `uint`, `enum`, fixed-width `hex`, and lowercase
hexadecimal `bytes`; fields may add enum values, inclusive integer bounds, or
the `unavailable` sentinel. Rust reply bindings accept `bytes` as borrowed byte
slices and encode them without allocation. Request bindings and generated Go do
not yet support `bytes`.

Validate schemas and generate checked-in Go or Rust bindings with:

```sh
cmdwire schema check schema/cmdwire/*.json
cmdwire schema generate-go cmdschema internal/cmdschema/generated.go \
  schema/cmdwire/*.json
cmdwire schema generate-rust src/cmdschema/generated.rs \
  schema/cmdwire/*.json
```

Generated Rust bindings depend only on the public `no_std` cmdwire crate. Schema
files and generated product bindings remain in the consuming repository.
Generated request decoders and reply types own command names, resources, field
order, counts, value encodings, and declared error constants. Implementations
supply typed operational values. Independent consumers validate replies from
the same schema documents. Product profiles carry exact hardware values and do
not redefine protocol structure.

Format version 1 represents exact record sequences. Format version 2 adds an
optional `occurs` object to reply records. `minimum` sets the required number of
ordered occurrences; an omitted `maximum` permits an unbounded stream. Generated
Rust bindings expose one bounded `push_<resource>` method per record group and a
`finish` method that verifies cardinality and emits the terminal with the actual
record count. Adjacent variable groups must have distinct kinds or resources so
consumers can classify their wire records without backtracking. Observation
lifecycles require a later schema extension; the wire protocol already supports
them.

## 9. Collection behavior

Before sending `request`, a consumer marks its position in the input stream. It
then collects matching data records, ignores unrelated lines, and stops at the
first matching terminal.

After `ok`, the consumer validates the count and command schema before exposing
buffered data. After `err`, it discards buffered data and exposes the stable
code and diagnostic fields. Consumers MUST bound buffered records or process
them incrementally. A timeout without a terminal is a local collection failure,
not an implicit protocol terminal.

## 10. Security and resource limits

Implementations MUST enforce the 80-byte limit before parsing fields. They MUST
bound buffered records, value lengths, and chunk totals. They SHOULD bound
observation duration when the command permits it. Applications MUST validate
resource paths before mapping them to filesystems, URLs, or other external
namespaces.

cmdwire provides no authentication, integrity, or confidentiality. A parser
cannot distinguish a protocol record from identical non-protocol text. Use a
trusted or authenticated transport when records cross a security boundary.
Parsing MUST NOT execute commands; schema validation, authorization, and
dispatch remain separate operations.

## 11. Conformance data

`testdata/conformance.json` is the language-neutral parser corpus.
Implementations should accept every valid case, produce the stated ordered
record model, and reject every invalid case. The corpus supplements this
specification, which remains authoritative.

## Implementation architecture

The Go implementation has five layers:

1. `lexer.go` enforces wire-level line and token rules, then classifies tokens.
2. [`grammar.y`](grammar.y) defines the Go record grammar over those tokens.
3. `validate.go` enforces record semantics that the grammar cannot express.
4. `Collector` validates counts across multiple records.
5. Command schemas validate request and reply resources, fields, and values.

The Rust crate implements the same normative ABNF as a bounded, borrowing
parser. It uses no allocator or runtime dependencies. Both parsers run the
language-neutral conformance corpus. The schema generator emits typed Rust
request decoders, aggregate command dispatch, bounded fixed replies, streaming
format-2 replies, and declared error encoders.

The record grammar does not parse individual wire bytes. It receives tokens only
after the lexer has validated and classified them. Semantic validation then
checks constraints such as duplicate fields and canonical numbers.

The repository includes generated `parser_gen.go`. Only maintainers need goyacc.
Regenerate the parser with the pinned Go tool:

```sh
go generate ./...
```

The normative wire grammar is the ABNF above. `grammar.y`, `lexer.go`, and
`validate.go` together implement that contract for Go.

## Verification

`testdata/conformance.json` provides a language-neutral parser corpus. Unit and
fuzz tests cover lexical boundaries and malformed input.

Run the standard checks with:

```sh
go generate ./...
git diff --exit-code -- parser_gen.go
go test ./...
go vet ./...
cargo test --locked --manifest-path rust/cmdwire/Cargo.toml --all-features
cargo clippy --locked --manifest-path rust/cmdwire/Cargo.toml --all-targets --all-features -- -D warnings
cargo check --locked --manifest-path rust/cmdwire/Cargo.toml --target aarch64-unknown-none
```

Run the parser fuzzer, then summarize its cached corpus:

```sh
go test -fuzz=FuzzParseLine -fuzztime=60s .
./scripts/fuzz-report
```

`FuzzFormat`, `FuzzDecoder`, `FuzzCollector`, and `FuzzParseSchema` cover the
remaining layers. The report summarizes the parser corpus, including rejection
paths, boundary inputs, and canonicalization examples.

Compare the Go and Rust parsers across the conformance corpus and the cached Go
parser fuzz corpus:

```sh
./scripts/differential-fuzz
```

Run a two-hour campaign for each fuzz target:

```sh
./scripts/fuzz-campaign
```

The campaign uses seven fuzz workers by default. Override its defaults with
`FUZZ_TIME`, `FUZZ_TIMEOUT`, and `FUZZ_PARALLEL`. On macOS, prevent sleep with
`caffeinate -i ./scripts/fuzz-campaign`.

The optional Gherkin suite tests public parsing and collection behavior. Its
independent module keeps Godog out of the package dependency graph.

```sh
(cd features && go test ./...)
```

Run mutation testing with:

```sh
./scripts/mutation-check
```

The script downloads its pinned Gremlins version on demand and uses seven
workers by default. Configure it through `MUTATION_*` environment variables:

```sh
MUTATION_WORKERS=4 MUTATION_TEST_CPU=1 ./scripts/mutation-check
MUTATION_DRY_RUN=1 ./scripts/mutation-check
MUTATION_INTEGRATION=1 MUTATION_OUTPUT=/tmp/mutation.json \
  ./scripts/mutation-check
```

Resource controls are `MUTATION_WORKERS`, `MUTATION_TEST_CPU`, and
`MUTATION_TIMEOUT_COEFFICIENT`. Quality gates are `MUTATION_EFFICACY` and
`MUTATION_COVERAGE`. Scope and reporting controls include `MUTATION_DIFF`,
`MUTATION_OUTPUT`, `MUTATION_OUTPUT_STATUSES`, `MUTATION_TAGS`, and
`MUTATION_COVERPKG`. `MUTATION_OPERATORS` accepts a comma-separated list of
additional operators.

`.gremlins.yaml` excludes generated code. CI runs the standard Go and Rust,
Gherkin, 10-second fuzz, differential parser, and mutation suites separately.

---

# Contributing

Before submitting a change:

```sh
gofmt -w *.go cmd/cmdwire/*.go features/*.go internal/differential/*.go \
  internal/schemagen/*.go
go generate ./...
git diff --exit-code -- parser_gen.go
go test -race ./...
go vet ./...
(cd features && go test ./...)
rustfmt --edition 2024 --check rust/cmdwire/src/lib.rs \
  rust/cmdwire/src/bin/parser_oracle.rs \
  rust/cmdwire/tests/{conformance,encoding}.rs
rustfmt --edition 2024 --config skip_children=true --check \
  rust/cmdwire/tests/generated_bindings.rs
cargo test --locked --manifest-path rust/cmdwire/Cargo.toml --all-features
cargo clippy --locked --manifest-path rust/cmdwire/Cargo.toml --all-targets --all-features -- -D warnings
cargo check --locked --manifest-path rust/cmdwire/Cargo.toml --target aarch64-unknown-none
./scripts/mutation-check
```

For wire-syntax changes, update the protocol specification in this README,
`grammar.y`, the generated parser, and the language-neutral conformance corpus
in one commit. For schema-format changes, update schema validation, generation,
tests, and this README together.

For lifecycle changes, update the specification and its Gherkin scenario.
Regenerate generated code instead of editing it.

Use lowercase imperative commit subjects such as:

```text
parser: reject repeated fields
```

Protocol behavior remains draft until the first stable release. Explain the
compatibility impact of each proposed wire change.
