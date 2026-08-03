package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadInputsIncludesLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conformance.json")
	data := []byte(`{"valid":[{"line":"request object.status"}],"invalid":[]}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	inputs, err := loadInputs(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"request object.status":     true,
		"request object.status\n":   true,
		"request object.status\r\n": true,
	}
	if len(inputs) != len(want) {
		t.Fatalf("inputs = %q", inputs)
	}
	for _, input := range inputs {
		if !want[input] {
			t.Fatalf("unexpected input %q", input)
		}
	}
}

func TestLoadInputsReportsFailures(t *testing.T) {
	validConformance := filepath.Join(t.TempDir(), "conformance.json")
	if err := os.WriteFile(validConformance, []byte(`{"valid":[],"invalid":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	invalidCorpus := t.TempDir()
	if err := os.WriteFile(filepath.Join(invalidCorpus, "bad"), []byte("not a fuzz entry"), 0o600); err != nil {
		t.Fatal(err)
	}
	brokenCorpus := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(brokenCorpus, "broken")); err != nil {
		t.Fatal(err)
	}

	for name, test := range map[string]struct {
		conformance string
		corpora     []string
	}{
		"missing conformance": {filepath.Join(t.TempDir(), "missing"), nil},
		"invalid conformance": {writeTestFile(t, "invalid.json", "{"), nil},
		"missing corpus":      {validConformance, []string{filepath.Join(t.TempDir(), "missing")}},
		"unreadable entry":    {validConformance, []string{brokenCorpus}},
		"invalid entry":       {validConformance, []string{invalidCorpus}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadInputs(test.conformance, test.corpora); err == nil {
				t.Fatal("loadInputs succeeded")
			}
		})
	}
}

func TestDecodeCorpusEntry(t *testing.T) {
	input, err := decodeCorpusEntry([]byte("go test fuzz v1\nstring(\"item object.status Alpha state=ready\")\n"))
	if err != nil {
		t.Fatal(err)
	}
	if input != "item object.status Alpha state=ready" {
		t.Fatalf("input = %q", input)
	}
}

func TestDecodeCorpusEntryRejectsMalformedData(t *testing.T) {
	for _, data := range []string{
		"bad header\nstring(\"value\")\n",
		"go test fuzz v1\nbytes(\"value\")\n",
		"go test fuzz v1\nstring(\"unterminated)\n",
	} {
		if _, err := decodeCorpusEntry([]byte(data)); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
}

func TestRunOracle(t *testing.T) {
	emptyOracle := writeScript(t, "empty-oracle", "exit 0")
	results, err := runOracle(emptyOracle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Fatalf("empty results = %q", results)
	}

	tooMany := writeScript(t, "too-many", "printf 'reject\\nreject\\n'")
	if _, err := runOracle(tooMany, []string{"bad"}); err == nil {
		t.Fatal("wrong result count accepted")
	}
	failed := writeScript(t, "failed", "echo failed >&2; exit 1")
	if _, err := runOracle(failed, []string{"bad"}); err == nil {
		t.Fatal("failed oracle accepted")
	}
}

func TestRun(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run(nil, &stdout, &stderr); status != 2 {
		t.Fatalf("usage status = %d", status)
	}

	corpus := t.TempDir()
	oracle := writeScript(t, "reject-oracle", "while IFS= read -r line; do echo reject; done")
	invalid := writeTestFile(t, "invalid-conformance.json", `{"valid":[],"invalid":[{"line":"bad"}]}`)
	stdout.Reset()
	stderr.Reset()
	if status := run([]string{oracle, invalid, corpus}, &stdout, &stderr); status != 0 {
		t.Fatalf("success status = %d, stderr = %q", status, stderr.String())
	}
	if stdout.String() != "Go and Rust agree on 3 parser inputs\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}

	valid := writeTestFile(t, "valid-conformance.json", `{"valid":[{"line":"request object.status"}],"invalid":[]}`)
	stdout.Reset()
	stderr.Reset()
	if status := run([]string{oracle, valid, corpus}, &stdout, &stderr); status != 1 {
		t.Fatalf("mismatch status = %d", status)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("parser mismatch")) {
		t.Fatalf("stderr = %q", stderr.String())
	}

	if status := run([]string{oracle, filepath.Join(t.TempDir(), "missing"), corpus}, &stdout, &stderr); status != 1 {
		t.Fatalf("load failure status = %d", status)
	}
	if status := run([]string{filepath.Join(t.TempDir(), "missing"), invalid, corpus}, &stdout, &stderr); status != 1 {
		t.Fatalf("oracle failure status = %d", status)
	}
}

func TestDescribe(t *testing.T) {
	const want = "accept\titem\tobject.status\tAlpha\tstate=ready"
	for _, input := range []string{
		"item object.status Alpha state=ready",
		"item object.status Alpha state=ready\n",
		"item object.status Alpha state=ready\r\n",
	} {
		if got := describe(input); got != want {
			t.Fatalf("description for %q = %q", input, got)
		}
	}
	if got := describe("not a record"); got != "reject" {
		t.Fatalf("rejected description = %q", got)
	}
}

func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	return writeExecutable(t, name, "#!/bin/sh\n"+body+"\n")
}

func writeExecutable(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
