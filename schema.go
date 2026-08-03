package cmdwire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Schema describes one command's request and successful reply contract.
type Schema struct {
	Format  uint64        `json:"format"`
	Command string        `json:"command"`
	Version uint64        `json:"version"`
	Request MessageSchema `json:"request"`
	Reply   ReplySchema   `json:"reply"`
	Errors  []ErrorSchema `json:"errors,omitempty"`
}

// MessageSchema describes one request record.
type MessageSchema struct {
	Resource string        `json:"resource,omitempty"`
	Fields   []FieldSchema `json:"fields,omitempty"`
}

// ReplySchema describes an ordered successful reply.
type ReplySchema struct {
	Records  []RecordSchema `json:"records,omitempty"`
	Terminal []FieldSchema  `json:"terminal,omitempty"`
}

// RecordSchema describes one ordered reply data record.
type RecordSchema struct {
	Kind     Kind          `json:"kind"`
	Resource string        `json:"resource,omitempty"`
	Fields   []FieldSchema `json:"fields,omitempty"`
	Occurs   *OccursSchema `json:"occurs,omitempty"`
}

// OccursSchema describes the permitted cardinality of one ordered reply record
// group. An absent maximum means the group is unbounded.
type OccursSchema struct {
	Minimum uint64  `json:"minimum"`
	Maximum *uint64 `json:"maximum,omitempty"`
}

// ErrorSchema describes one stable error code and its diagnostic fields.
type ErrorSchema struct {
	Code   string        `json:"code"`
	Fields []FieldSchema `json:"fields,omitempty"`
}

// FieldSchema describes one ordered field and its wire value constraints.
type FieldSchema struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required,omitempty"`
	Values      []string `json:"values,omitempty"`
	Width       int      `json:"width,omitempty"`
	Minimum     *uint64  `json:"minimum,omitempty"`
	Maximum     *uint64  `json:"maximum,omitempty"`
	Unavailable bool     `json:"unavailable,omitempty"`
}

// DecodeSchema decodes and validates one schema document.
func DecodeSchema(input io.Reader) (schema Schema, err error) {
	decoder := json.NewDecoder(input)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&schema); err != nil {
		return Schema{}, fmt.Errorf("cmdwire: decoding schema: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("trailing JSON value")
		}
		return Schema{}, fmt.Errorf("cmdwire: decoding schema: %w", err)
	}
	if err = schema.Validate(); err != nil {
		return Schema{}, err
	}
	return
}

// ParseSchema parses and validates one schema document.
func ParseSchema(input []byte) (Schema, error) {
	return DecodeSchema(bytes.NewReader(input))
}

// Validate validates a schema document independently of any record.
func (schema Schema) Validate() error {
	if schema.Format != 1 && schema.Format != 2 {
		return fmt.Errorf("cmdwire: unsupported schema format %d", schema.Format)
	}
	if !validCommand(schema.Command) {
		return fmt.Errorf("cmdwire: invalid schema command %q", schema.Command)
	}
	if schema.Version == 0 {
		return fmt.Errorf("cmdwire: schema version must be positive")
	}
	if err := validateMessageSchema("request", schema.Request); err != nil {
		return err
	}
	for index, record := range schema.Reply.Records {
		if record.Kind != Event && record.Kind != Item && record.Kind != Chunk {
			return fmt.Errorf("cmdwire: reply record %d has invalid kind %q", index, record.Kind)
		}
		if record.Resource != "" && !validResource(record.Resource) {
			return fmt.Errorf("cmdwire: reply record %d has invalid resource %q", index, record.Resource)
		}
		if err := validateFieldSchemas(fmt.Sprintf("reply record %d", index), record.Fields); err != nil {
			return err
		}
		if (record.Kind == Event || record.Kind == Chunk) && len(record.Fields) == 0 {
			return fmt.Errorf("cmdwire: reply record %d requires fields", index)
		}
		if record.Occurs != nil {
			if schema.Format == 1 {
				return fmt.Errorf("cmdwire: reply record %d uses occurs in schema format 1", index)
			}
			if record.Occurs.Maximum != nil && (*record.Occurs.Maximum == 0 || *record.Occurs.Maximum < record.Occurs.Minimum) {
				return fmt.Errorf("cmdwire: reply record %d has invalid occurrence bounds", index)
			}
		}
		if index > 0 && sameRecordIdentity(schema.Reply.Records[index-1], record) &&
			(schema.Reply.Records[index-1].Occurs != nil || record.Occurs != nil) {
			return fmt.Errorf("cmdwire: adjacent variable reply records %d and %d are ambiguous", index-1, index)
		}
	}
	if err := validateFieldSchemas("reply terminal", schema.Reply.Terminal); err != nil {
		return err
	}
	for _, field := range schema.Reply.Terminal {
		if field.Name == "schema" || field.Name == "count" {
			return fmt.Errorf("cmdwire: reply terminal field %q is reserved", field.Name)
		}
	}
	seenCodes := make(map[string]bool, len(schema.Errors))
	for _, errorSchema := range schema.Errors {
		if !validErrorCode(errorSchema.Code) || seenCodes[errorSchema.Code] {
			return fmt.Errorf("cmdwire: invalid or repeated schema error code %q", errorSchema.Code)
		}
		if err := validateFieldSchemas("error "+errorSchema.Code, errorSchema.Fields); err != nil {
			return err
		}
		for _, field := range errorSchema.Fields {
			if field.Name == "code" {
				return fmt.Errorf("cmdwire: error %s field %q is reserved", errorSchema.Code, field.Name)
			}
		}
		seenCodes[errorSchema.Code] = true
	}
	return nil
}

// ValidateRequest checks one request against the command schema.
func (schema Schema) ValidateRequest(record Record) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if record.Kind != Request {
		return fmt.Errorf("cmdwire: %s is not a request", record.Kind)
	}
	if record.Command != schema.Command {
		return fmt.Errorf("cmdwire: request command is %s, schema is %s", record.Command, schema.Command)
	}
	if record.Resource != schema.Request.Resource {
		return fmt.Errorf("cmdwire: request resource is %q, want %q", record.Resource, schema.Request.Resource)
	}
	return validateSchemaFields("request", schema.Request.Fields, record.Fields)
}

// ValidateError checks one error terminal against the command schema.
func (schema Schema) ValidateError(record Record) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if record.Kind != Error {
		return fmt.Errorf("cmdwire: %s is not an error", record.Kind)
	}
	if record.Command != schema.Command {
		return fmt.Errorf("cmdwire: error command is %s, schema is %s", record.Command, schema.Command)
	}
	if _, err := Format(record); err != nil {
		return err
	}
	code := record.Fields[0].Value
	for _, errorSchema := range schema.Errors {
		if code == errorSchema.Code {
			return validateSchemaFields("error "+code, errorSchema.Fields, record.Fields[1:])
		}
	}
	return fmt.Errorf("cmdwire: error code %s is not allowed by %s", code, schema.Command)
}

// ValidateReply checks one collected successful reply against the schema.
func (schema Schema) ValidateReply(report Report) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if report.Command != schema.Command {
		return fmt.Errorf("cmdwire: reply command is %s, schema is %s", report.Command, schema.Command)
	}
	if report.Schema != schema.Version {
		return fmt.Errorf("cmdwire: reply schema is %d, want %d", report.Schema, schema.Version)
	}
	position := 0
	for index, want := range schema.Reply.Records {
		minimum, maximum := recordOccurrenceBounds(want)
		count := uint64(0)
		for position < len(report.Data) && sameRecord(report.Data[position], want) && (maximum == nil || count < *maximum) {
			if err := validateSchemaFields(fmt.Sprintf("reply record %d occurrence %d", index, count), want.Fields, report.Data[position].Fields); err != nil {
				return err
			}
			position++
			count++
		}
		if count < minimum {
			return fmt.Errorf("cmdwire: reply record %d occurs %d times, minimum is %d", index, count, minimum)
		}
	}
	if position != len(report.Data) {
		return fmt.Errorf("cmdwire: reply has unexpected data record %d", position)
	}
	if len(report.Terminal) < 2 {
		return fmt.Errorf("cmdwire: reply terminal lacks schema and count")
	}
	terminal := Record{Kind: OK, Command: report.Command, Fields: report.Terminal}
	if _, err := Format(terminal); err != nil {
		return fmt.Errorf("cmdwire: invalid reply terminal: %w", err)
	}
	if report.Terminal[0].Value != strconv.FormatUint(report.Schema, 10) {
		return fmt.Errorf("cmdwire: reply terminal schema is %s, report schema is %d", report.Terminal[0].Value, report.Schema)
	}
	count, _ := strconv.ParseUint(report.Terminal[1].Value, 10, 64)
	if count != uint64(len(report.Data)) {
		return fmt.Errorf("cmdwire: reply terminal count is %d, data count is %d", count, len(report.Data))
	}
	return validateSchemaFields("reply terminal", schema.Reply.Terminal, report.Terminal[2:])
}

func recordOccurrenceBounds(record RecordSchema) (uint64, *uint64) {
	if record.Occurs != nil {
		return record.Occurs.Minimum, record.Occurs.Maximum
	}
	one := uint64(1)
	return one, &one
}

func sameRecord(record Record, schema RecordSchema) bool {
	return record.Kind == schema.Kind && record.Resource == schema.Resource
}

func sameRecordIdentity(left, right RecordSchema) bool {
	return left.Kind == right.Kind && left.Resource == right.Resource
}

func validateMessageSchema(name string, schema MessageSchema) error {
	if schema.Resource != "" && !validResource(schema.Resource) {
		return fmt.Errorf("cmdwire: %s has invalid resource %q", name, schema.Resource)
	}
	return validateFieldSchemas(name, schema.Fields)
}

func validateFieldSchemas(context string, fields []FieldSchema) error {
	seen := make(map[string]bool, len(fields))
	optional := false
	for _, field := range fields {
		if !validFieldName(field.Name) || seen[field.Name] {
			return fmt.Errorf("cmdwire: %s has invalid or repeated field %q", context, field.Name)
		}
		seen[field.Name] = true
		if optional && field.Required {
			return fmt.Errorf("cmdwire: %s has required field %q after an optional field", context, field.Name)
		}
		optional = optional || !field.Required
		switch field.Type {
		case "token", "bool", "bytes":
			if len(field.Values) != 0 || field.Width != 0 || field.Minimum != nil || field.Maximum != nil {
				return fmt.Errorf("cmdwire: %s field %q has constraints invalid for %s", context, field.Name, field.Type)
			}
		case "uint":
			if len(field.Values) != 0 || field.Width != 0 {
				return fmt.Errorf("cmdwire: %s field %q has constraints invalid for uint", context, field.Name)
			}
		case "enum":
			if len(field.Values) == 0 || field.Width != 0 || field.Minimum != nil || field.Maximum != nil {
				return fmt.Errorf("cmdwire: %s enum field %q has invalid constraints", context, field.Name)
			}
			values := make(map[string]bool, len(field.Values))
			for _, value := range field.Values {
				if !validValue(value) || values[value] {
					return fmt.Errorf("cmdwire: %s enum field %q has invalid value %q", context, field.Name, value)
				}
				values[value] = true
			}
		case "hex":
			if field.Width <= 0 || len(field.Values) != 0 || field.Minimum != nil || field.Maximum != nil {
				return fmt.Errorf("cmdwire: %s hex field %q has invalid constraints", context, field.Name)
			}
		default:
			return fmt.Errorf("cmdwire: %s field %q has invalid type %q", context, field.Name, field.Type)
		}
		if field.Minimum != nil && field.Maximum != nil && *field.Minimum > *field.Maximum {
			return fmt.Errorf("cmdwire: %s field %q has minimum greater than maximum", context, field.Name)
		}
	}
	return nil
}

func validateSchemaFields(context string, schema []FieldSchema, fields []Field) error {
	position := 0
	for _, want := range schema {
		if position == len(fields) || fields[position].Name != want.Name {
			if want.Required {
				return fmt.Errorf("cmdwire: %s is missing required field %q", context, want.Name)
			}
			continue
		}
		if err := validateSchemaValue(want, fields[position].Value); err != nil {
			return fmt.Errorf("cmdwire: %s field %q: %w", context, want.Name, err)
		}
		position++
	}
	if position != len(fields) {
		return fmt.Errorf("cmdwire: %s has unexpected field %q", context, fields[position].Name)
	}
	return nil
}

func validateSchemaValue(schema FieldSchema, value string) error {
	if schema.Unavailable && value == "unavailable" {
		return nil
	}
	switch schema.Type {
	case "token":
		if !validValue(value) {
			return fmt.Errorf("invalid token")
		}
	case "bool":
		if value != "true" && value != "false" {
			return fmt.Errorf("invalid boolean %q", value)
		}
	case "bytes":
		if value == "" || len(value)%2 != 0 {
			return fmt.Errorf("invalid byte string %q", value)
		}
		for _, char := range value {
			if !digit(char) && (char < 'a' || char > 'f') {
				return fmt.Errorf("invalid byte string %q", value)
			}
		}
	case "uint":
		if !validDecimal(value, false) {
			return fmt.Errorf("invalid unsigned integer %q", value)
		}
		number, _ := strconv.ParseUint(value, 10, 64)
		if schema.Minimum != nil && number < *schema.Minimum {
			return fmt.Errorf("value %d is below %d", number, *schema.Minimum)
		}
		if schema.Maximum != nil && number > *schema.Maximum {
			return fmt.Errorf("value %d exceeds %d", number, *schema.Maximum)
		}
	case "enum":
		for _, allowed := range schema.Values {
			if value == allowed {
				return nil
			}
		}
		return fmt.Errorf("invalid enum value %q", value)
	case "hex":
		if len(value) != schema.Width+2 || len(value) < 3 || value[:2] != "0x" {
			return fmt.Errorf("invalid %d-digit hexadecimal value %q", schema.Width, value)
		}
		for _, char := range value[2:] {
			if !digit(char) && (char < 'a' || char > 'f') {
				return fmt.Errorf("invalid %d-digit hexadecimal value %q", schema.Width, value)
			}
		}
	default:
		return fmt.Errorf("unsupported type %q", schema.Type)
	}
	return nil
}
