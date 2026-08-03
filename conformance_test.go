package cmdwire

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type conformanceCorpus struct {
	Version int `json:"version"`
	Valid   []struct {
		Line   string `json:"line"`
		Record Record `json:"record"`
	} `json:"valid"`
	Invalid []struct {
		Line string `json:"line"`
	} `json:"invalid"`
}

func loadConformance(t *testing.T) conformanceCorpus {
	t.Helper()
	data, err := os.ReadFile("testdata/conformance.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var corpus conformanceCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != 1 {
		t.Fatalf("conformance version = %d, want 1", corpus.Version)
	}
	return corpus
}

func TestConformanceValid(t *testing.T) {
	for _, test := range loadConformance(t).Valid {
		t.Run(test.Line, func(t *testing.T) {
			record, err := ParseLine(test.Line)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(record, test.Record) {
				t.Fatalf("record = %#v, want %#v", record, test.Record)
			}
			line, err := Format(record)
			if err != nil {
				t.Fatal(err)
			}
			if line != test.Line {
				t.Fatalf("canonical line = %q, want %q", line, test.Line)
			}
		})
	}
}

func TestConformanceInvalid(t *testing.T) {
	for _, test := range loadConformance(t).Invalid {
		t.Run(test.Line, func(t *testing.T) {
			if record, err := ParseLine(test.Line); err == nil {
				t.Fatalf("ParseLine succeeded: %#v", record)
			}
		})
	}
}
