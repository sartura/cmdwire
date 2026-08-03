package cmdwire

import (
	"errors"
	"strings"
	"testing"
)

func TestParseLineEndings(t *testing.T) {
	for _, ending := range []string{"", "\n", "\r\n"} {
		record, err := ParseLine("ok object.status schema=1 count=0" + ending)
		if err != nil {
			t.Fatal(err)
		}
		if record.Kind != OK || record.Command != "object.status" {
			t.Fatalf("unexpected record: %#v", record)
		}
	}
	if _, err := ParseLine("ok object.status schema=1 count=0\r"); err == nil {
		t.Fatal("bare CR accepted")
	}
	if _, err := ParseLine("ok object.status schema=1 count=0\nitem object.status"); err == nil {
		t.Fatal("embedded line ending accepted")
	}
}

func TestLineWidthBoundary(t *testing.T) {
	prefix := "request a value="
	valid := prefix + strings.Repeat("a", MaxLineBytes-len(prefix))
	if len(valid) != MaxLineBytes {
		t.Fatalf("test line is %d bytes", len(valid))
	}
	if _, err := ParseLine(valid); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLine(valid + "a"); err == nil {
		t.Fatal("oversized record accepted")
	}
}

func TestResourceSyntax(t *testing.T) {
	for _, line := range []string{
		"request object.status alpha1.beta-2",
		"item object.status 0:1/alpha_beta.gamma",
	} {
		if _, err := ParseLine(line); err != nil {
			t.Fatalf("ParseLine(%q): %v", line, err)
		}
	}
	for _, line := range []string{
		"request object.status ..",
		"request object.status .alpha",
		"request object.status alpha.",
		"request object.status alpha..beta",
		"request object.status alpha.beta.gamma",
		"request object.status -",
		"request object.status :::::::",
		"request object.status _alpha",
		"request object.status alpha_",
	} {
		if _, err := ParseLine(line); err == nil {
			t.Fatalf("ParseLine(%q) accepted invalid resource", line)
		}
	}
}

func TestValueSyntax(t *testing.T) {
	for _, line := range []string{
		"request object.set value=ready",
		"request object.set value=a/b:c._-",
		"request object.set value=a=b",
	} {
		if _, err := ParseLine(line); err != nil {
			t.Fatalf("ParseLine(%q): %v", line, err)
		}
	}
	for _, line := range []string{
		"request object.set value=",
		`request object.set value=""`,
		`request object.set value="ready"`,
		`request object.set value=a\b`,
		"request object.set value=caf\u00e9",
		"request object.set value=two words",
	} {
		if _, err := ParseLine(line); err == nil {
			t.Fatalf("ParseLine(%q) accepted invalid value", line)
		}
	}
	for _, value := range []string{"", "two words", "quote\"", `back\slash`, "caf\u00e9", "\n"} {
		_, err := Format(Record{
			Kind: Request, Command: "object.set",
			Fields: []Field{{Name: "value", Value: value}},
		})
		if err == nil {
			t.Fatalf("Format accepted invalid value %q", value)
		}
	}
}

func TestNoticeStructure(t *testing.T) {
	for _, line := range []string{
		"notice system.boot state=ready",
		"notice system.boot storage state=initializing",
	} {
		if _, err := ParseLine(line); err != nil {
			t.Fatalf("ParseLine(%q): %v", line, err)
		}
	}
	for _, line := range []string{
		"notice system.boot",
		"notice system.boot storage",
	} {
		if _, err := ParseLine(line); err == nil {
			t.Fatalf("ParseLine(%q) accepted empty notice", line)
		}
	}
}

func TestSuccessStructure(t *testing.T) {
	for _, line := range []string{
		"ok object.status schema=1 count=0",
		"ok object.status schema=1 count=2 stop=count",
		"ok object.status schema=18446744073709551615 count=18446744073709551615",
	} {
		if _, err := ParseLine(line); err != nil {
			t.Fatalf("ParseLine(%q): %v", line, err)
		}
	}
	for _, line := range []string{
		"ok object.status",
		"ok object.status schema=1",
		"ok object.status count=0 schema=1",
		"ok object.status state=ready schema=1 count=0",
		"ok object.status schema=0 count=0",
		"ok object.status schema=01 count=0",
		"ok object.status schema=1 count=00",
		"ok object.status schema=1 count=+1",
		"ok object.status schema=18446744073709551616 count=0",
		"ok object.status schema=1 count=18446744073709551616",
	} {
		if _, err := ParseLine(line); err == nil {
			t.Fatalf("ParseLine(%q) accepted invalid success", line)
		}
	}
}

func TestErrorStructure(t *testing.T) {
	for _, line := range []string{
		"err object.action code=BAD_VALUE",
		"err object.action code=BAD_VALUE field=mode",
	} {
		if _, err := ParseLine(line); err != nil {
			t.Fatalf("ParseLine(%q): %v", line, err)
		}
	}
	for _, line := range []string{
		"err object.action",
		"err object.action field=mode code=BAD_VALUE",
		"err object.action code=bad",
		"err object.action code=BAD code=AGAIN",
	} {
		if _, err := ParseLine(line); err == nil {
			t.Fatalf("ParseLine(%q) accepted invalid error", line)
		}
	}
}

func TestSyntaxErrorHasColumn(t *testing.T) {
	_, err := ParseLine("ok  object.status")
	var syntax *SyntaxError
	if !errors.As(err, &syntax) {
		t.Fatalf("error = %T, want SyntaxError", err)
	}
	if syntax.Column != 4 {
		t.Fatalf("column = %d, want 4", syntax.Column)
	}
}

func TestFormatCanonicalRecord(t *testing.T) {
	record := Record{
		Kind: Request, Command: "object.set", Resource: "alpha",
		Fields: []Field{{Name: "value", Value: "a/b:c._-"}},
	}
	line, err := Format(record)
	if err != nil {
		t.Fatal(err)
	}
	if line != "request object.set alpha value=a/b:c._-" {
		t.Fatalf("line = %q", line)
	}
	parsed, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := parsed.Field("value"); !ok || value != "a/b:c._-" {
		t.Fatalf("value = %q, present %v", value, ok)
	}
}

func TestFormatRejectsInvalidRecords(t *testing.T) {
	tests := []Record{
		{Kind: OK, Command: "Bad"},
		{Kind: Event, Command: "object.observe"},
		{Kind: Notice, Command: "system.boot"},
		{Kind: Request, Command: "object.status", Fields: []Field{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}},
		{Kind: OK, Command: "object.status", Resource: "alpha"},
		{Kind: OK, Command: "object.status", Fields: []Field{{Name: "schema", Value: "1"}}},
		{Kind: Error, Command: "object.status"},
	}
	for _, record := range tests {
		if line, err := Format(record); err == nil {
			t.Fatalf("Format(%#v) = %q", record, line)
		}
	}
}

func TestFormatRejectsOversizedRecord(t *testing.T) {
	_, err := Format(Record{
		Kind: Request, Command: "object.status",
		Fields: []Field{{Name: "value", Value: strings.Repeat("a", 80)}},
	})
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("error = %v", err)
	}
}

func TestSyntaxErrorFormatting(t *testing.T) {
	for _, test := range []struct {
		column int
		want   string
	}{
		{column: -1, want: "cmdwire: broken"},
		{column: 0, want: "cmdwire: broken"},
		{column: 1, want: "cmdwire: column 1: broken"},
	} {
		err := (&SyntaxError{Column: test.column, Message: "broken"}).Error()
		if err != test.want {
			t.Errorf("column %d: error = %q, want %q", test.column, err, test.want)
		}
	}
}

func TestFieldRejectsAmbiguousConstructedRecord(t *testing.T) {
	record := Record{Fields: []Field{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}}
	if value, ok := record.Field("x"); ok || value != "" {
		t.Fatalf("Field = %q, %v", value, ok)
	}
}
