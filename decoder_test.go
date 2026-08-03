package cmdwire

import (
	"io"
	"strings"
	"testing"
)

func TestDecoder(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(
		"request object.status\r\n" +
			"ok object.status schema=1 count=0 state=ready\n" +
			"err object.observe code=FAILED",
	))
	for index, kind := range []Kind{Request, OK, Error} {
		record, err := decoder.Decode()
		if err != nil {
			t.Fatalf("record %d: %v", index, err)
		}
		if record.Kind != kind {
			t.Fatalf("record %d kind = %s, want %s", index, record.Kind, kind)
		}
	}
	if _, err := decoder.Decode(); err != io.EOF {
		t.Fatalf("final error = %v, want EOF", err)
	}
}

func TestDecoderRecoversAfterOversizedLine(t *testing.T) {
	decoder := NewDecoder(strings.NewReader(
		strings.Repeat("x", MaxLineBytes+100) + "\n" +
			"ok object.status schema=1 count=0 state=ready\n",
	))
	if _, err := decoder.Decode(); err == nil {
		t.Fatal("oversized line accepted")
	}
	record, err := decoder.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != OK {
		t.Fatalf("kind = %s, want ok", record.Kind)
	}
}
