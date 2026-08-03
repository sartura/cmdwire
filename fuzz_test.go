package cmdwire

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func FuzzParseLine(f *testing.F) {
	for _, seed := range []string{
		"request object.status",
		"ok object.status schema=1 count=0 state=ready",
		"err object.status code=FAILED field=mode",
		"notice system.boot storage state=ready",
		"event object.observe state=waiting",
		"item object.status alpha state=ready",
		"chunk object.read offset=0 data=0011",
		`request a value="quoted"`,
		"request a value=caf\u00e9",
		"err a code=A detail=" + strings.Repeat("0", 58),
		"request a ..",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line string) {
		record, err := ParseLine(line)
		if err != nil {
			return
		}
		assertRecordRoundTrip(t, record)
	})
}

func FuzzFormat(f *testing.F) {
	f.Add("ok", "object.status", "", "schema", "1", "count", "0")
	f.Add("err", "object.status", "", "code", "FAILED", "field", "mode")
	f.Add("item", "object.status", "alpha.v1", "state", "ready", "", "")
	f.Add("notice", "system.boot", "storage", "state", "ready", "", "")
	f.Fuzz(func(t *testing.T, kind, command, resource, name1, value1, name2, value2 string) {
		record := Record{Kind: Kind(kind), Command: command, Resource: resource}
		if name1 != "" {
			record.Fields = append(record.Fields, Field{Name: name1, Value: value1})
		}
		if name2 != "" {
			record.Fields = append(record.Fields, Field{Name: name2, Value: value2})
		}
		if _, err := Format(record); err != nil {
			return
		}
		assertRecordRoundTrip(t, record)
	})
}

func FuzzDecoder(f *testing.F) {
	f.Add([]byte("request object.status\r\nok object.status schema=1 count=0 state=ready\n"))
	f.Add([]byte(strings.Repeat("x", MaxLineBytes+1) + "\nok a\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		decoder := NewDecoder(bytes.NewReader(input))
		for attempts := 0; attempts <= len(input)+1; attempts++ {
			record, err := decoder.Decode()
			if err != nil {
				return
			}
			assertRecordRoundTrip(t, record)
		}
		t.Fatal("decoder did not consume bounded input")
	})
}

func FuzzCollector(f *testing.F) {
	f.Add("object.status", "notice object.status lifecycle state=ready\nitem object.status alpha state=ready\nok object.status schema=1 count=1")
	f.Add("object.status", "noise\nerr object.status code=FAILED field=mode")
	f.Fuzz(func(t *testing.T, command, transcript string) {
		collector, err := NewCollectorWithLimit(command, 8)
		if err != nil {
			return
		}
		for _, line := range strings.Split(transcript, "\n") {
			_, complete, err := collector.AddLine(line)
			if err != nil {
				return
			}
			if complete {
				_, _ = collector.Result()
				return
			}
		}
		_, _ = collector.Result()
	})
}

func FuzzParseSchema(f *testing.F) {
	f.Add([]byte(statusSchemaJSON))
	f.Add([]byte(`{"format":1,"command":"test.status","version":1,"request":{},"reply":{}}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		schema, err := ParseSchema(input)
		if err != nil {
			return
		}
		if err := schema.Validate(); err != nil {
			t.Fatalf("parsed schema is invalid: %v", err)
		}
	})
}

func assertRecordRoundTrip(t *testing.T, record Record) {
	t.Helper()
	formatted, err := Format(record)
	if err != nil {
		t.Fatalf("record cannot be formatted: %v", err)
	}
	roundTrip, err := ParseLine(formatted)
	if err != nil {
		t.Fatalf("formatted record cannot be parsed: %v", err)
	}
	if !reflect.DeepEqual(roundTrip, record) {
		t.Fatalf("round trip changed record:\noriginal:  %#v\nround trip: %#v", record, roundTrip)
	}
	formattedAgain, err := Format(roundTrip)
	if err != nil {
		t.Fatalf("round-trip record cannot be formatted: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("format is not idempotent: %q != %q", formattedAgain, formatted)
	}
}
