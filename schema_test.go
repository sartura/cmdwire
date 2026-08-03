package cmdwire

import (
	"strings"
	"testing"
)

const statusSchemaJSON = `{
  "format": 1,
  "command": "object.status",
  "version": 1,
  "request": {},
  "reply": {
    "records": [
      {
        "kind": "item",
        "resource": "control",
        "fields": [
          {"name": "state", "type": "enum", "required": true, "values": ["active", "inactive"]},
          {"name": "mode", "type": "enum", "required": true, "values": ["automatic", "manual"]},
          {"name": "healthy", "type": "bool", "required": true}
        ]
      },
      {
        "kind": "item",
        "resource": "metrics",
        "fields": [
          {"name": "limit", "type": "uint", "required": true},
          {"name": "counter", "type": "hex", "required": true, "width": 8}
        ]
      }
    ]
  },
  "errors": [{"code": "BAD_REQUEST", "fields": [{"name": "field", "type": "token"}]}]
}`

func TestSchemaValidatesRequestAndReply(t *testing.T) {
	schema, err := ParseSchema([]byte(statusSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	request, err := ParseLine("request object.status")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateRequest(request); err != nil {
		t.Fatal(err)
	}

	collector, err := NewCollector("object.status")
	if err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"item object.status control state=active mode=automatic healthy=true",
		"item object.status metrics limit=15 counter=0xffffffff",
		"ok object.status schema=1 count=2",
	}
	for _, line := range lines {
		if _, _, err := collector.AddLine(line); err != nil {
			t.Fatal(err)
		}
	}
	report, err := collector.Result()
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateReply(report); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaRejectsInvalidReplyValue(t *testing.T) {
	schema, err := ParseSchema([]byte(statusSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	report := Report{
		Command: "object.status",
		Schema:  1,
		Data: []Record{
			{Kind: Item, Command: "object.status", Resource: "control", Fields: []Field{
				{Name: "state", Value: "unknown"},
				{Name: "mode", Value: "automatic"},
				{Name: "healthy", Value: "true"},
			}},
			{Kind: Item, Command: "object.status", Resource: "metrics", Fields: []Field{
				{Name: "limit", Value: "15"},
				{Name: "counter", Value: "0xffffffff"},
			}},
		},
		Terminal: []Field{{Name: "schema", Value: "1"}, {Name: "count", Value: "2"}},
	}
	if err := schema.ValidateReply(report); err == nil || !strings.Contains(err.Error(), "invalid enum") {
		t.Fatalf("error = %v", err)
	}
}

func TestSchemaValidatesDeclaredError(t *testing.T) {
	schema, err := ParseSchema([]byte(statusSchemaJSON))
	if err != nil {
		t.Fatal(err)
	}
	record, err := ParseLine("err object.status code=BAD_REQUEST field=mode")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.ValidateError(record); err != nil {
		t.Fatal(err)
	}
	record.Fields[1].Name = "detail"
	if err := schema.ValidateError(record); err == nil {
		t.Fatal("undeclared error diagnostic accepted")
	}
	record.Fields[0].Value = "UNSUPPORTED"
	if err := schema.ValidateError(record); err == nil {
		t.Fatal("undeclared error code accepted")
	}
}

func TestDecodeSchemaRejectsUnknownAndTrailingData(t *testing.T) {
	for _, input := range []string{
		`{"format":1,"command":"x","version":1,"request":{},"reply":{},"unknown":true}`,
		`{"format":1,"command":"x","version":1,"request":{},"reply":{}} {}`,
	} {
		if _, err := ParseSchema([]byte(input)); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestSchemaValidatesVariableReplyRecords(t *testing.T) {
	maximum := uint64(3)
	schema := Schema{
		Format: 2, Command: "object.list", Version: 1,
		Reply: ReplySchema{Records: []RecordSchema{{
			Kind: Item, Resource: "entry",
			Fields: []FieldSchema{{Name: "name", Type: "token", Required: true}},
			Occurs: &OccursSchema{Minimum: 1, Maximum: &maximum},
		}}},
	}
	report := Report{
		Command: "object.list", Schema: 1,
		Data: []Record{
			{Kind: Item, Command: "object.list", Resource: "entry", Fields: []Field{{Name: "name", Value: "alpha"}}},
			{Kind: Item, Command: "object.list", Resource: "entry", Fields: []Field{{Name: "name", Value: "beta"}}},
		},
		Terminal: []Field{{Name: "schema", Value: "1"}, {Name: "count", Value: "2"}},
	}
	if err := schema.ValidateReply(report); err != nil {
		t.Fatal(err)
	}
	report.Data = nil
	if err := schema.ValidateReply(report); err == nil || !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("minimum error = %v", err)
	}
}

func TestSchemaRejectsMalformedReplyTerminal(t *testing.T) {
	schema := Schema{Format: 1, Command: "object.status", Version: 1}
	for name, terminal := range map[string][]Field{
		"wrong field names": {{Name: "version", Value: "1"}, {Name: "total", Value: "0"}},
		"mismatched schema": {{Name: "schema", Value: "2"}, {Name: "count", Value: "0"}},
		"mismatched count":  {{Name: "schema", Value: "1"}, {Name: "count", Value: "1"}},
	} {
		t.Run(name, func(t *testing.T) {
			report := Report{Command: "object.status", Schema: 1, Terminal: terminal}
			if err := schema.ValidateReply(report); err == nil {
				t.Fatal("malformed terminal accepted")
			}
		})
	}
}

func TestSchemaValidatesByteFields(t *testing.T) {
	schema := FieldSchema{Name: "data", Type: "bytes", Required: true}
	for _, value := range []string{"00", "deadbeef"} {
		if err := validateSchemaValue(schema, value); err != nil {
			t.Fatalf("value %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "0", "ABC0", "gg"} {
		if err := validateSchemaValue(schema, value); err == nil {
			t.Fatalf("invalid value %q accepted", value)
		}
	}
}

func TestSchemaRejectsOccursInFormatOne(t *testing.T) {
	schema := Schema{
		Format: 1, Command: "object.list", Version: 1,
		Reply: ReplySchema{Records: []RecordSchema{{
			Kind: Item, Occurs: &OccursSchema{},
		}}},
	}
	if err := schema.Validate(); err == nil || !strings.Contains(err.Error(), "format 1") {
		t.Fatalf("error = %v", err)
	}
}

func TestSchemaRejectsAmbiguousAdjacentVariableRecords(t *testing.T) {
	maximum := uint64(2)
	schema := Schema{
		Format: 2, Command: "object.list", Version: 1,
		Reply: ReplySchema{Records: []RecordSchema{
			{Kind: Item, Resource: "entry", Occurs: &OccursSchema{Maximum: &maximum}},
			{Kind: Item, Resource: "entry"},
		}},
	}
	if err := schema.Validate(); err == nil || !strings.Contains(err.Error(), "records 0 and 1") {
		t.Fatalf("error = %v", err)
	}
}

func TestSchemaRejectsReservedTerminalFields(t *testing.T) {
	for _, name := range []string{"schema", "count"} {
		schema := Schema{
			Format: 1, Command: "object.status", Version: 1,
			Reply: ReplySchema{Terminal: []FieldSchema{{Name: name, Type: "token"}}},
		}
		if err := schema.Validate(); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("field %q error = %v", name, err)
		}
	}
}

func TestSchemaRejectsRequiredFieldAfterOptional(t *testing.T) {
	schema := Schema{
		Format: 1, Command: "test.command", Version: 1,
		Request: MessageSchema{Fields: []FieldSchema{
			{Name: "optional", Type: "token"},
			{Name: "required", Type: "token", Required: true},
		}},
	}
	if err := schema.Validate(); err == nil {
		t.Fatal("required field after optional accepted")
	}
}
