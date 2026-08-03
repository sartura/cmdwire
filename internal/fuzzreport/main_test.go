package main

import (
	"errors"
	"testing"
)

func TestDecodeCorpusEntry(t *testing.T) {
	input, err := decodeCorpusEntry([]byte("go test fuzz v1\nstring(\"ok a state=ready\")\n"))
	if err != nil {
		t.Fatal(err)
	}
	if input != "ok a state=ready" {
		t.Fatalf("input = %q", input)
	}
}

func TestDecodeCorpusEntryRejectsUnsupportedData(t *testing.T) {
	if _, err := decodeCorpusEntry([]byte("not a corpus entry")); err == nil {
		t.Fatal("unsupported corpus entry accepted")
	}
}

func TestClassifyError(t *testing.T) {
	tests := map[string]string{
		"cmdwire: invalid resource path \"..\"":      "invalid resource path",
		"cmdwire: field \"value\" has invalid value": "invalid field value",
		"cmdwire: unrelated failure":                 "other syntax error",
	}
	for message, want := range tests {
		if got := classifyError(errors.New(message)); got != want {
			t.Errorf("classifyError(%q) = %q, want %q", message, got, want)
		}
	}
}
