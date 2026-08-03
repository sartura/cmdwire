package schemagen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sartura/cmdwire"
)

func TestGoGeneratesTypedRequestAndReply(t *testing.T) {
	schema := cmdwire.Schema{
		Format: 1, Command: "object.status", Version: 1,
		Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{{
			Kind: cmdwire.Item, Resource: "state",
			Fields: []cmdwire.FieldSchema{
				{Name: "available", Type: "bool", Required: true},
				{Name: "counter", Type: "hex", Width: 8, Required: true},
			},
		}}},
		Errors: []cmdwire.ErrorSchema{{Code: "BAD_REQUEST"}},
	}
	generated, err := Go("cmdschema", []cmdwire.Schema{schema})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, want := range []string{
		"type ObjectStatusRequest struct",
		"type ObjectStatusReply struct",
		"StateAvailable bool",
		"StateCounter",
		`ErrorObjectStatusBadRequest ObjectStatusErrorCode = "BAD_REQUEST"`,
		`Resource: "state"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated source lacks %q:\n%s", want, text)
		}
	}
}

func TestGoGeneratesAllFieldShapes(t *testing.T) {
	minimum, maximum := uint64(2), uint64(9)
	schema := cmdwire.Schema{
		Format: 1, Command: "test.all", Version: 2,
		Request: cmdwire.MessageSchema{Fields: []cmdwire.FieldSchema{
			{Name: "enabled", Type: "bool", Required: true},
			{Name: "limit", Type: "uint", Required: true},
			{Name: "address", Type: "hex", Width: 8, Required: true},
			{Name: "name", Type: "token", Required: true},
			{Name: "optional", Type: "token"},
		}},
		Reply: cmdwire.ReplySchema{
			Records: []cmdwire.RecordSchema{{
				Kind: cmdwire.Item,
				Fields: []cmdwire.FieldSchema{
					{Name: "state", Type: "enum", Values: []string{"ready", "busy"}, Required: true},
					{Name: "count", Type: "uint", Minimum: &minimum, Maximum: &maximum, Required: true},
					{Name: "address", Type: "hex", Width: 8, Required: true},
					{Name: "sample", Type: "uint", Maximum: &maximum, Unavailable: true, Required: true},
					{Name: "healthy", Type: "bool"},
				}}},
			Terminal: []cmdwire.FieldSchema{
				{Name: "total", Type: "uint", Maximum: &maximum, Required: true},
				{Name: "mode", Type: "enum", Values: []string{"fast", "safe"}},
			},
		},
	}
	generated, err := Go("cmdschema", []cmdwire.Schema{schema})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, want := range []string{
		"Optional *string",
		"Record1Healthy *bool",
		"TerminalMode",
		"request.Address = parsed",
		"case \"ready\", \"busy\":",
		"if reply.Record1Count < 2",
		"if reply.Record1Count > 9",
		"if reply.Record1Address > 0xffffffff",
		"Record1SampleValue := \"unavailable\"",
		"fmt.Sprintf(\"0x%08x\", reply.Record1Address)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated source lacks %q:\n%s", want, text)
		}
	}

	terminalOnly := cmdwire.Schema{
		Format: 1, Command: "test.terminal", Version: 1,
		Reply: cmdwire.ReplySchema{Terminal: []cmdwire.FieldSchema{{Name: "address", Type: "hex", Width: 8}}},
	}
	terminalGenerated, err := Go("cmdschema", []cmdwire.Schema{terminalOnly})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(terminalGenerated), `"fmt"`) {
		t.Fatalf("terminal generated source does not import fmt:\n%s", terminalGenerated)
	}
}

func TestGoRejectsUnsupportedGeneratedFields(t *testing.T) {
	for name, schema := range map[string]cmdwire.Schema{
		"reply bytes": {
			Format: 1, Command: "test.reply", Version: 1,
			Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{{
				Kind:   cmdwire.Item,
				Fields: []cmdwire.FieldSchema{{Name: "data", Type: "bytes"}},
			}}},
		},
		"terminal unavailable bool": {
			Format: 1, Command: "test.terminal", Version: 1,
			Reply: cmdwire.ReplySchema{Terminal: []cmdwire.FieldSchema{{Name: "state", Type: "bool", Unavailable: true}}},
		},
		"wide terminal hex": {
			Format: 1, Command: "test.wide", Version: 1,
			Reply: cmdwire.ReplySchema{Terminal: []cmdwire.FieldSchema{{Name: "address", Type: "hex", Width: 17}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Go("cmdschema", []cmdwire.Schema{schema}); err == nil {
				t.Fatal("unsupported field accepted")
			}
		})
	}
}

func TestGoRejectsDuplicateCommand(t *testing.T) {
	schema := cmdwire.Schema{Format: 1, Command: "test.command", Version: 1}
	if _, err := Go("cmdschema", []cmdwire.Schema{schema, schema}); err == nil {
		t.Fatal("duplicate command accepted")
	}
}

func TestGoMinimalSchemaCompiles(t *testing.T) {
	schema := cmdwire.Schema{Format: 1, Command: "object.status", Version: 1}
	generated, err := Go("cmdschema", []cmdwire.Schema{schema})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), `"fmt"`) {
		t.Fatalf("minimal generated source imports fmt:\n%s", generated)
	}

	directory := t.TempDir()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	module := fmt.Sprintf("module generated.test\n\ngo 1.26.4\n\nrequire github.com/sartura/cmdwire v0.0.0\n\nreplace github.com/sartura/cmdwire => %s\n", filepath.ToSlash(root))
	if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "generated.go"), generated, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "test", ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compiling minimal generated source: %v\n%s", err, output)
	}
}

func TestGoRejectsCollidingGeneratedNames(t *testing.T) {
	field := cmdwire.FieldSchema{Name: "state", Type: "token", Required: true}
	for name, schemas := range map[string][]cmdwire.Schema{
		"commands": {
			{Format: 1, Command: "test.alpha-v1", Version: 1},
			{Format: 1, Command: "test.alpha.v1", Version: 1},
		},
		"request fields": {{
			Format: 1, Command: "test.request", Version: 1,
			Request: cmdwire.MessageSchema{Fields: []cmdwire.FieldSchema{
				{Name: "alpha_1", Type: "token"},
				{Name: "alpha1", Type: "token"},
			}},
		}},
		"same reply identity": {{
			Format: 1, Command: "test.same", Version: 1,
			Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{
				{Kind: cmdwire.Item, Resource: "alpha", Fields: []cmdwire.FieldSchema{field}},
				{Kind: cmdwire.Item, Resource: "alpha", Fields: []cmdwire.FieldSchema{field}},
			}},
		}},
		"normalized reply identity": {{
			Format: 1, Command: "test.normalized", Version: 1,
			Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{
				{Kind: cmdwire.Item, Resource: "alpha-v1", Fields: []cmdwire.FieldSchema{field}},
				{Kind: cmdwire.Item, Resource: "alpha.v1", Fields: []cmdwire.FieldSchema{field}},
			}},
		}},
		"reply and terminal fields": {{
			Format: 1, Command: "test.terminal", Version: 1,
			Reply: cmdwire.ReplySchema{
				Records:  []cmdwire.RecordSchema{{Kind: cmdwire.Item, Resource: "terminal", Fields: []cmdwire.FieldSchema{field}}},
				Terminal: []cmdwire.FieldSchema{{Name: "state", Type: "token"}},
			},
		}},
		"error codes": {{
			Format: 1, Command: "test.errors", Version: 1,
			Errors: []cmdwire.ErrorSchema{{Code: "BAD_VALUE"}, {Code: "BAD__VALUE"}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Go("cmdschema", schemas); err == nil || !strings.Contains(err.Error(), "collides") {
				t.Fatalf("error = %v, want generated-name collision", err)
			}
		})
	}
}

func TestRustGeneratesNoStdTypedBindings(t *testing.T) {
	maximum := uint64(100)
	schema := cmdwire.Schema{
		Format: 1, Command: "object.status", Version: 1,
		Request: cmdwire.MessageSchema{Fields: []cmdwire.FieldSchema{
			{Name: "limit", Type: "uint", Maximum: &maximum},
		}},
		Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{{
			Kind: cmdwire.Item, Resource: "state",
			Fields: []cmdwire.FieldSchema{
				{Name: "available", Type: "bool", Required: true},
				{Name: "counter", Type: "hex", Width: 8, Required: true},
			},
		}}},
		Errors: []cmdwire.ErrorSchema{{Code: "BAD_REQUEST"}},
	}
	generated, err := Rust([]cmdwire.Schema{schema})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, want := range []string{
		"// Code generated by cmdwire; DO NOT EDIT.",
		"pub struct ObjectStatusRequest",
		"pub struct ObjectStatusReply",
		"pub enum ObjectStatusError",
		"::cmdwire::decode_uint",
		"::cmdwire::encode_record",
		"::cmdwire::WireField::hex",
		"pub enum DecodedCommand",
		"pub fn decode_command",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated Rust source lacks %q:\n%s", want, text)
		}
	}
}

func TestRustGeneratesFixedAndStreamingReplies(t *testing.T) {
	minimum, maximum := uint64(1), uint64(2)
	fixed := cmdwire.Schema{
		Format: 1, Command: "test.fixed", Version: 1,
		Reply: cmdwire.ReplySchema{
			Records: []cmdwire.RecordSchema{{
				Kind:   cmdwire.Item,
				Fields: []cmdwire.FieldSchema{{Name: "name", Type: "token", Required: true}},
			}},
			Terminal: []cmdwire.FieldSchema{{Name: "complete", Type: "bool", Required: true}},
		},
	}
	streaming := cmdwire.Schema{
		Format: 2, Command: "test.stream", Version: 1,
		Request: cmdwire.MessageSchema{Fields: []cmdwire.FieldSchema{
			{Name: "enabled", Type: "bool", Required: true},
			{Name: "limit", Type: "uint"},
		}},
		Reply: cmdwire.ReplySchema{
			Records: []cmdwire.RecordSchema{
				{
					Kind: cmdwire.Item, Resource: "first", Occurs: &cmdwire.OccursSchema{Minimum: minimum, Maximum: &maximum},
					Fields: []cmdwire.FieldSchema{
						{Name: "name", Type: "token", Required: true},
						{Name: "healthy", Type: "bool"},
					},
				},
				{
					Kind:   cmdwire.Chunk,
					Fields: []cmdwire.FieldSchema{{Name: "data", Type: "bytes", Required: true}},
				},
				{
					Kind: cmdwire.Item, Resource: "last", Occurs: &cmdwire.OccursSchema{Minimum: minimum},
					Fields: []cmdwire.FieldSchema{{Name: "sample", Type: "uint", Unavailable: true, Required: true}},
				},
			},
			Terminal: []cmdwire.FieldSchema{
				{Name: "mode", Type: "enum", Values: []string{"fast", "safe"}, Required: true},
				{Name: "address", Type: "hex", Width: 8},
			},
		},
		Errors: []cmdwire.ErrorSchema{{
			Code:   "BAD_VALUE",
			Fields: []cmdwire.FieldSchema{{Name: "field", Type: "token", Required: true}},
		}},
	}
	withoutTerminal := cmdwire.Schema{
		Format: 2, Command: "test.empty", Version: 1,
		Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{{
			Kind: cmdwire.Item, Resource: "entry", Occurs: &cmdwire.OccursSchema{},
		}}},
	}
	generated, err := Rust([]cmdwire.Schema{fixed, streaming, withoutTerminal})
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, want := range []string{
		"pub terminal_complete: bool",
		"pub struct TestStreamReplyRecord2<'a>",
		"pub fn push_record_2<'a>",
		"self.occurrences[0] < 1",
		"self.occurrences[2] != 0",
		"self.occurrences[0] >= 2",
		"pub fn finish<'a>(self, terminal_fields: TestStreamReplyTerminal<'a>)",
		"pub fn finish(self) -> Result<::cmdwire::Line",
		"::cmdwire::Available::Unavailable",
		"::cmdwire::WireField::bytes",
		"&[\"fast\", \"safe\"]",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated Rust source lacks %q:\n%s", want, text)
		}
	}

	if got := rustFieldName("1.alpha"); got != "_1_alpha" {
		t.Fatalf("rust field name = %q", got)
	}
	if got := rustStringSlice([]string{"alpha", "beta"}); got != "&[\"alpha\", \"beta\"]" {
		t.Fatalf("Rust string slice = %q", got)
	}
}

func TestRustRejectsUnsupportedReplyAndErrorFields(t *testing.T) {
	wide := cmdwire.FieldSchema{Name: "address", Type: "hex", Width: 17}
	for name, schema := range map[string]cmdwire.Schema{
		"reply": {
			Format: 1, Command: "test.reply", Version: 1,
			Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{{Kind: cmdwire.Item, Fields: []cmdwire.FieldSchema{wide}}}},
		},
		"error": {
			Format: 1, Command: "test.error", Version: 1,
			Errors: []cmdwire.ErrorSchema{{Code: "BAD_VALUE", Fields: []cmdwire.FieldSchema{wide}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Rust([]cmdwire.Schema{schema}); err == nil {
				t.Fatal("unsupported field accepted")
			}
		})
	}
}

func TestRustRejectsDuplicateCommand(t *testing.T) {
	schema := cmdwire.Schema{Format: 1, Command: "test.command", Version: 1}
	if _, err := Rust([]cmdwire.Schema{schema, schema}); err == nil {
		t.Fatal("duplicate command accepted")
	}
}

func TestRustRejectsCollidingGeneratedNames(t *testing.T) {
	first := cmdwire.Schema{Format: 1, Command: "test.foo-bar", Version: 1}
	second := cmdwire.Schema{Format: 1, Command: "test.foo_bar", Version: 1}
	if _, err := Rust([]cmdwire.Schema{first, second}); err == nil {
		t.Fatal("colliding generated command names accepted")
	}

	fieldCollision := cmdwire.Schema{
		Format: 1, Command: "test.fields", Version: 1,
		Request: cmdwire.MessageSchema{Fields: []cmdwire.FieldSchema{
			{Name: "type", Type: "token"},
			{Name: "type_", Type: "token"},
		}},
	}
	if _, err := Rust([]cmdwire.Schema{fieldCollision}); err == nil {
		t.Fatal("colliding generated field names accepted")
	}

	field := cmdwire.FieldSchema{Name: "state", Type: "token", Required: true}
	replyCollision := cmdwire.Schema{
		Format: 1, Command: "test.reply", Version: 1,
		Reply: cmdwire.ReplySchema{Records: []cmdwire.RecordSchema{
			{Kind: cmdwire.Item, Resource: "alpha-v1", Fields: []cmdwire.FieldSchema{field}},
			{Kind: cmdwire.Item, Resource: "alpha.v1", Fields: []cmdwire.FieldSchema{field}},
		}},
	}
	if _, err := Rust([]cmdwire.Schema{replyCollision}); err == nil {
		t.Fatal("colliding generated reply field names accepted")
	}

	terminalCollision := cmdwire.Schema{
		Format: 1, Command: "test.terminal", Version: 1,
		Reply: cmdwire.ReplySchema{
			Records:  []cmdwire.RecordSchema{{Kind: cmdwire.Item, Resource: "terminal", Fields: []cmdwire.FieldSchema{field}}},
			Terminal: []cmdwire.FieldSchema{{Name: "state", Type: "token"}},
		},
	}
	if _, err := Rust([]cmdwire.Schema{terminalCollision}); err == nil {
		t.Fatal("colliding terminal field name accepted")
	}
}
