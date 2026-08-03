#![no_std]

//! Allocation-free parsing and encoding for the cmdwire line protocol.

use core::fmt;
use core::fmt::Write as _;

/// Maximum record width, excluding the line ending.
pub const MAX_LINE_BYTES: usize = 80;

/// A cmdwire record kind.
#[non_exhaustive]
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Kind {
    Request,
    Ok,
    Error,
    Notice,
    Event,
    Item,
    Chunk,
}

impl Kind {
    /// Return the wire token for this kind.
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Request => "request",
            Self::Ok => "ok",
            Self::Error => "err",
            Self::Notice => "notice",
            Self::Event => "event",
            Self::Item => "item",
            Self::Chunk => "chunk",
        }
    }
}

/// One borrowed field from a parsed record.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct Field<'a> {
    pub name: &'a str,
    pub value: &'a str,
}

/// An iterator over the ordered fields of a parsed record.
#[derive(Clone, Debug)]
pub struct Fields<'a> {
    remaining: &'a str,
}

impl<'a> Fields<'a> {
    const fn new(remaining: &'a str) -> Self {
        Self { remaining }
    }
}

impl<'a> Iterator for Fields<'a> {
    type Item = Field<'a>;

    fn next(&mut self) -> Option<Self::Item> {
        if self.remaining.is_empty() {
            return None;
        }
        let (token, rest) = match self.remaining.split_once(' ') {
            Some(parts) => parts,
            None => (self.remaining, ""),
        };
        self.remaining = rest;
        let (name, value) = token.split_once('=')?;
        Some(Field { name, value })
    }
}

/// One syntactically and semantically valid borrowed record.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct Record<'a> {
    kind: Kind,
    command: &'a str,
    resource: Option<&'a str>,
    fields: &'a str,
}

impl<'a> Record<'a> {
    pub const fn kind(&self) -> Kind {
        self.kind
    }

    pub const fn command(&self) -> &'a str {
        self.command
    }

    pub const fn resource(&self) -> Option<&'a str> {
        self.resource
    }

    pub const fn fields(&self) -> Fields<'a> {
        Fields::new(self.fields)
    }

    pub fn field(&self, name: &str) -> Option<&'a str> {
        self.fields()
            .find(|field| field.name == name)
            .map(|field| field.value)
    }
}

/// Broad parse failure categories suitable for bounded firmware diagnostics.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ParseErrorKind {
    Empty,
    TooLong,
    NonAscii,
    Spacing,
    UnknownKind,
    MissingCommand,
    InvalidCommand,
    InvalidResource,
    InvalidField,
    DuplicateField,
    InvalidRecord,
}

/// A parse error with a one-based byte column where available.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ParseError {
    pub column: usize,
    pub kind: ParseErrorKind,
}

impl ParseError {
    const fn new(column: usize, kind: ParseErrorKind) -> Self {
        Self { column, kind }
    }
}

impl fmt::Display for ParseError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.column == 0 {
            write!(formatter, "cmdwire: {:?}", self.kind)
        } else {
            write!(
                formatter,
                "cmdwire: column {}: {:?}",
                self.column, self.kind
            )
        }
    }
}

/// Parse one record line from bytes. The input may omit its line ending or end
/// in LF or CRLF.
pub fn parse_line_bytes(input: &[u8]) -> Result<Record<'_>, ParseError> {
    parse_bytes(strip_line_ending(input))
}

/// Parse one record line. The input may omit its line ending or end in LF or
/// CRLF.
pub fn parse_line(input: &str) -> Result<Record<'_>, ParseError> {
    parse_line_bytes(input.as_bytes())
}

/// Parse one record body from bytes without a line ending.
pub fn parse_bytes(input: &[u8]) -> Result<Record<'_>, ParseError> {
    validate_physical_record(input)?;
    // SAFETY: validate_physical_record accepts printable ASCII bytes only.
    let input = unsafe { core::str::from_utf8_unchecked(input) };
    parse_validated(input)
}

/// Parse one record body without a line ending.
pub fn parse(input: &str) -> Result<Record<'_>, ParseError> {
    validate_physical_record(input.as_bytes())?;
    parse_validated(input)
}

fn strip_line_ending(input: &[u8]) -> &[u8] {
    input
        .strip_suffix(b"\r\n")
        .or_else(|| input.strip_suffix(b"\n"))
        .unwrap_or(input)
}

fn validate_physical_record(input: &[u8]) -> Result<(), ParseError> {
    if input.is_empty() {
        return Err(ParseError::new(1, ParseErrorKind::Empty));
    }
    if input.len() > MAX_LINE_BYTES {
        return Err(ParseError::new(MAX_LINE_BYTES + 1, ParseErrorKind::TooLong));
    }
    let mut previous_space = false;
    for (index, byte) in input.iter().copied().enumerate() {
        if !(0x20..=0x7e).contains(&byte) {
            return Err(ParseError::new(index + 1, ParseErrorKind::NonAscii));
        }
        let space = byte == b' ';
        if space && (index == 0 || index + 1 == input.len() || previous_space) {
            return Err(ParseError::new(1, ParseErrorKind::Spacing));
        }
        previous_space = space;
    }
    Ok(())
}

fn parse_validated(input: &str) -> Result<Record<'_>, ParseError> {
    let mut tokens = Tokens::new(input);
    let (_, kind_token) = tokens
        .next()
        .ok_or(ParseError::new(1, ParseErrorKind::Empty))?;
    let kind = match kind_token {
        "request" => Kind::Request,
        "ok" => Kind::Ok,
        "err" => Kind::Error,
        "notice" => Kind::Notice,
        "event" => Kind::Event,
        "item" => Kind::Item,
        "chunk" => Kind::Chunk,
        _ => return Err(ParseError::new(1, ParseErrorKind::UnknownKind)),
    };
    let (command_offset, command) = tokens.next().ok_or(ParseError::new(
        input.len() + 1,
        ParseErrorKind::MissingCommand,
    ))?;
    if !valid_command(command) {
        return Err(ParseError::new(
            command_offset + 1,
            ParseErrorKind::InvalidCommand,
        ));
    }

    let mut resource = None;
    if matches!(
        kind,
        Kind::Request | Kind::Notice | Kind::Event | Kind::Item | Kind::Chunk
    ) && let Some((offset, token)) = tokens.peek()
        && !token.contains('=')
    {
        tokens.next();
        if !valid_resource(token) {
            return Err(ParseError::new(offset + 1, ParseErrorKind::InvalidResource));
        }
        resource = Some(token);
    }

    let fields_offset = tokens.offset;
    let fields = if fields_offset < input.len() {
        &input[fields_offset..]
    } else {
        ""
    };
    validate_fields(fields, fields_offset)?;
    validate_record(kind, fields, fields_offset)?;

    Ok(Record {
        kind,
        command,
        resource,
        fields,
    })
}

#[derive(Clone)]
struct Tokens<'a> {
    input: &'a str,
    offset: usize,
}

impl<'a> Tokens<'a> {
    const fn new(input: &'a str) -> Self {
        Self { input, offset: 0 }
    }

    fn next(&mut self) -> Option<(usize, &'a str)> {
        if self.offset >= self.input.len() {
            return None;
        }
        let start = self.offset;
        let rest = &self.input[start..];
        let token = match rest.find(' ') {
            Some(length) => {
                self.offset = start + length + 1;
                &rest[..length]
            }
            None => {
                self.offset = self.input.len();
                rest
            }
        };
        Some((start, token))
    }

    fn peek(&self) -> Option<(usize, &'a str)> {
        self.clone().next()
    }
}

fn validate_fields(fields: &str, base: usize) -> Result<(), ParseError> {
    let mut position = base;
    for field in Fields::new(fields) {
        if !valid_field_name(field.name) || !valid_value(field.value) {
            return Err(ParseError::new(position + 1, ParseErrorKind::InvalidField));
        }
        let duplicates = Fields::new(fields)
            .filter(|candidate| candidate.name == field.name)
            .count();
        if duplicates != 1 {
            return Err(ParseError::new(
                position + 1,
                ParseErrorKind::DuplicateField,
            ));
        }
        position += field.name.len() + field.value.len() + 2;
    }
    if !fields.is_empty() && Fields::new(fields).count() != fields.split(' ').count() {
        return Err(ParseError::new(base + 1, ParseErrorKind::InvalidField));
    }
    Ok(())
}

fn validate_record(kind: Kind, fields: &str, base: usize) -> Result<(), ParseError> {
    let mut fields = Fields::new(fields);
    match kind {
        Kind::Request | Kind::Item => {}
        Kind::Notice | Kind::Event | Kind::Chunk => {
            if fields.next().is_none() {
                return Err(ParseError::new(base + 1, ParseErrorKind::InvalidRecord));
            }
        }
        Kind::Ok => {
            let schema = fields
                .next()
                .ok_or(ParseError::new(base + 1, ParseErrorKind::InvalidRecord))?;
            let count = fields
                .next()
                .ok_or(ParseError::new(base + 1, ParseErrorKind::InvalidRecord))?;
            if schema.name != "schema"
                || !valid_decimal(schema.value, true)
                || count.name != "count"
                || !valid_decimal(count.value, false)
            {
                return Err(ParseError::new(base + 1, ParseErrorKind::InvalidRecord));
            }
        }
        Kind::Error => {
            let code = fields
                .next()
                .ok_or(ParseError::new(base + 1, ParseErrorKind::InvalidRecord))?;
            if code.name != "code" || !valid_error_code(code.value) {
                return Err(ParseError::new(base + 1, ParseErrorKind::InvalidRecord));
            }
        }
    }
    Ok(())
}

fn valid_command(command: &str) -> bool {
    !command.is_empty()
        && command.split('.').all(|segment| {
            let mut bytes = segment.bytes();
            matches!(bytes.next(), Some(b'a'..=b'z'))
                && bytes.all(|byte| {
                    byte.is_ascii_lowercase()
                        || byte.is_ascii_digit()
                        || byte == b'_'
                        || byte == b'-'
                })
        })
}

fn valid_resource(resource: &str) -> bool {
    !resource.is_empty()
        && resource.split('/').all(|segment| {
            let mut parts = segment.split('.');
            let first = parts.next().is_some_and(valid_resource_part);
            let second = parts.next().is_none_or(valid_resource_part);
            first && second && parts.next().is_none()
        })
}

fn valid_resource_part(part: &str) -> bool {
    let bytes = part.as_bytes();
    !bytes.is_empty()
        && bytes[0].is_ascii_alphanumeric()
        && bytes[bytes.len() - 1].is_ascii_alphanumeric()
        && bytes
            .iter()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'_' | b':' | b'-'))
}

fn valid_field_name(name: &str) -> bool {
    let mut bytes = name.bytes();
    matches!(bytes.next(), Some(b'a'..=b'z'))
        && bytes.all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'_')
}

fn valid_value(value: &str) -> bool {
    !value.is_empty()
        && value
            .bytes()
            .all(|byte| (b'!'..=b'~').contains(&byte) && !matches!(byte, b'"' | b'\\'))
}

fn valid_error_code(code: &str) -> bool {
    let mut bytes = code.bytes();
    matches!(bytes.next(), Some(b'A'..=b'Z'))
        && bytes.all(|byte| byte.is_ascii_uppercase() || byte.is_ascii_digit() || byte == b'_')
}

fn valid_decimal(value: &str, positive: bool) -> bool {
    if value == "0" {
        return !positive;
    }
    matches!(value.as_bytes().first(), Some(b'1'..=b'9'))
        && value.bytes().skip(1).all(|byte| byte.is_ascii_digit())
        && value.parse::<u64>().is_ok()
}

/// A schema-level request decoding error.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum DecodeError {
    WrongKind,
    WrongCommand,
    UnknownCommand,
    WrongResource,
    MissingField(&'static str),
    UnexpectedField,
    InvalidField(&'static str),
}

/// A value that may be represented by the protocol's `unavailable` sentinel.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Available<T> {
    Value(T),
    Unavailable,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum WireValue<'a> {
    Token(&'a str),
    Bytes(&'a [u8]),
    Bool(bool),
    Uint(u64),
    Hex(u64, usize),
}

/// One field to be encoded into a record.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct WireField<'a> {
    name: &'a str,
    value: WireValue<'a>,
}

impl<'a> WireField<'a> {
    pub const fn token(name: &'a str, value: &'a str) -> Self {
        Self {
            name,
            value: WireValue::Token(value),
        }
    }

    pub const fn bytes(name: &'a str, value: &'a [u8]) -> Self {
        Self {
            name,
            value: WireValue::Bytes(value),
        }
    }

    pub const fn boolean(name: &'a str, value: bool) -> Self {
        Self {
            name,
            value: WireValue::Bool(value),
        }
    }

    pub const fn uint(name: &'a str, value: u64) -> Self {
        Self {
            name,
            value: WireValue::Uint(value),
        }
    }

    pub const fn hex(name: &'a str, value: u64, width: usize) -> Self {
        Self {
            name,
            value: WireValue::Hex(value, width),
        }
    }
}

/// An encoding error detected before bytes reach a transport.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum EncodeError {
    InvalidRecord,
    InvalidField(&'static str),
    InvalidReplyState,
    TooLong,
}

/// One encoded physical record without a line ending.
#[derive(Clone, Copy, Eq, PartialEq)]
pub struct Line {
    bytes: [u8; MAX_LINE_BYTES],
    length: usize,
}

impl Line {
    const fn new() -> Self {
        Self {
            bytes: [0; MAX_LINE_BYTES],
            length: 0,
        }
    }

    pub fn as_str(&self) -> &str {
        core::str::from_utf8(&self.bytes[..self.length]).expect("encoded cmdwire line is ASCII")
    }

    fn push(&mut self, value: &str) -> Result<(), EncodeError> {
        let end = self
            .length
            .checked_add(value.len())
            .ok_or(EncodeError::TooLong)?;
        if end > MAX_LINE_BYTES {
            return Err(EncodeError::TooLong);
        }
        self.bytes[self.length..end].copy_from_slice(value.as_bytes());
        self.length = end;
        Ok(())
    }
}

impl fmt::Debug for Line {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.debug_tuple("Line").field(&self.as_str()).finish()
    }
}

impl fmt::Display for Line {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str(self.as_str())
    }
}

struct LineWriter<'a>(&'a mut Line);

impl fmt::Write for LineWriter<'_> {
    fn write_str(&mut self, value: &str) -> fmt::Result {
        self.0.push(value).map_err(|_| fmt::Error)
    }
}

/// Encode one record into a fixed 80-byte line.
pub fn encode_record<'a, const N: usize>(
    kind: Kind,
    command: &'a str,
    resource: Option<&'a str>,
    fields: [Option<WireField<'a>>; N],
) -> Result<Line, EncodeError> {
    if !valid_command(command) || resource.is_some_and(|value| !valid_resource(value)) {
        return Err(EncodeError::InvalidRecord);
    }
    if resource.is_some()
        && !matches!(
            kind,
            Kind::Request | Kind::Notice | Kind::Event | Kind::Item | Kind::Chunk
        )
    {
        return Err(EncodeError::InvalidRecord);
    }

    let present = fields.iter().flatten().count();
    if matches!(kind, Kind::Notice | Kind::Event | Kind::Chunk) && present == 0 {
        return Err(EncodeError::InvalidRecord);
    }
    for (index, field) in fields.iter().flatten().enumerate() {
        if !valid_field_name(field.name) {
            return Err(EncodeError::InvalidRecord);
        }
        if fields
            .iter()
            .flatten()
            .skip(index + 1)
            .any(|other| other.name == field.name)
        {
            return Err(EncodeError::InvalidRecord);
        }
        if let WireValue::Token(value) = field.value
            && !valid_value(value)
        {
            return Err(EncodeError::InvalidRecord);
        }
        if let WireValue::Bytes(value) = field.value
            && value.is_empty()
        {
            return Err(EncodeError::InvalidRecord);
        }
        if let WireValue::Hex(value, width) = field.value
            && (width == 0 || width > 16 || (width < 16 && value >= (1_u64 << (width * 4))))
        {
            return Err(EncodeError::InvalidRecord);
        }
    }
    if kind == Kind::Ok {
        let mut fields = fields.iter().flatten();
        let schema = fields.next().ok_or(EncodeError::InvalidRecord)?;
        let count = fields.next().ok_or(EncodeError::InvalidRecord)?;
        if schema.name != "schema"
            || !matches!(schema.value, WireValue::Uint(value) if value > 0)
            || count.name != "count"
            || !matches!(count.value, WireValue::Uint(_))
        {
            return Err(EncodeError::InvalidRecord);
        }
    }
    if kind == Kind::Error {
        let code = fields
            .iter()
            .flatten()
            .next()
            .ok_or(EncodeError::InvalidRecord)?;
        if code.name != "code" {
            return Err(EncodeError::InvalidRecord);
        }
        match code.value {
            WireValue::Token(value) if valid_error_code(value) => {}
            _ => return Err(EncodeError::InvalidRecord),
        }
    }

    let mut line = Line::new();
    line.push(kind.as_str())?;
    line.push(" ")?;
    line.push(command)?;
    if let Some(resource) = resource {
        line.push(" ")?;
        line.push(resource)?;
    }
    for field in fields.into_iter().flatten() {
        line.push(" ")?;
        line.push(field.name)?;
        line.push("=")?;
        match field.value {
            WireValue::Token(value) => line.push(value)?,
            WireValue::Bytes(value) => {
                let mut writer = LineWriter(&mut line);
                for byte in value {
                    write!(&mut writer, "{byte:02x}").map_err(|_| EncodeError::TooLong)?;
                }
            }
            WireValue::Bool(value) => line.push(if value { "true" } else { "false" })?,
            WireValue::Uint(value) => {
                let mut writer = LineWriter(&mut line);
                write!(&mut writer, "{value}").map_err(|_| EncodeError::TooLong)?
            }
            WireValue::Hex(value, width) => {
                let mut writer = LineWriter(&mut line);
                write!(&mut writer, "0x{value:0width$x}").map_err(|_| EncodeError::TooLong)?
            }
        }
    }
    Ok(line)
}

/// Parse a canonical unsigned decimal field value.
pub fn decode_uint(
    name: &'static str,
    value: &str,
    minimum: Option<u64>,
    maximum: Option<u64>,
) -> Result<u64, DecodeError> {
    if !valid_decimal(value, false) {
        return Err(DecodeError::InvalidField(name));
    }
    let value = value
        .parse::<u64>()
        .map_err(|_| DecodeError::InvalidField(name))?;
    if minimum.is_some_and(|minimum| value < minimum)
        || maximum.is_some_and(|maximum| value > maximum)
    {
        return Err(DecodeError::InvalidField(name));
    }
    Ok(value)
}

/// Parse a canonical boolean field value.
pub fn decode_bool(name: &'static str, value: &str) -> Result<bool, DecodeError> {
    match value {
        "true" => Ok(true),
        "false" => Ok(false),
        _ => Err(DecodeError::InvalidField(name)),
    }
}

/// Parse a fixed-width lowercase hexadecimal field value.
pub fn decode_hex(name: &'static str, value: &str, width: usize) -> Result<u64, DecodeError> {
    let digits = value
        .strip_prefix("0x")
        .ok_or(DecodeError::InvalidField(name))?;
    if digits.len() != width
        || !digits
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err(DecodeError::InvalidField(name));
    }
    u64::from_str_radix(digits, 16).map_err(|_| DecodeError::InvalidField(name))
}

/// Validate one enum field value.
pub fn decode_enum<'a>(
    name: &'static str,
    value: &'a str,
    allowed: &[&str],
) -> Result<&'a str, DecodeError> {
    allowed
        .contains(&value)
        .then_some(value)
        .ok_or(DecodeError::InvalidField(name))
}

/// Validate a value before encoding a schema field.
pub fn validate_uint(
    name: &'static str,
    value: u64,
    minimum: Option<u64>,
    maximum: Option<u64>,
) -> Result<u64, EncodeError> {
    if minimum.is_some_and(|minimum| value < minimum)
        || maximum.is_some_and(|maximum| value > maximum)
    {
        return Err(EncodeError::InvalidField(name));
    }
    Ok(value)
}

/// Validate a fixed-width hexadecimal value before encoding a schema field.
pub fn validate_hex(name: &'static str, value: u64, width: usize) -> Result<u64, EncodeError> {
    if width == 0 || width > 16 || (width < 16 && value >= (1_u64 << (width * 4))) {
        return Err(EncodeError::InvalidField(name));
    }
    Ok(value)
}

/// Validate an enum value before encoding a schema field.
pub fn validate_enum<'a>(
    name: &'static str,
    value: &'a str,
    allowed: &[&str],
) -> Result<&'a str, EncodeError> {
    allowed
        .contains(&value)
        .then_some(value)
        .ok_or(EncodeError::InvalidField(name))
}
