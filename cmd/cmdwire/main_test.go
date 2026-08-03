package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run(nil, strings.NewReader(""), &stdout, &stderr); status != 2 {
		t.Fatalf("status = %d, want 2", status)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCheckAndFormat(t *testing.T) {
	input := "request object.status\nok object.status schema=1 count=0 state=ready\n"
	var stdout, stderr bytes.Buffer
	if status := run([]string{"check"}, strings.NewReader(input), &stdout, &stderr); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("check output = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := run(
		[]string{"format"}, strings.NewReader("request object.set value=a/b"),
		&stdout, &stderr,
	); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	if stdout.String() != "request object.set value=a/b\n" {
		t.Fatalf("format output = %q", stdout.String())
	}
}

func TestRunSchemaCheckAndGenerateGo(t *testing.T) {
	directory := t.TempDir()
	schemaPath := filepath.Join(directory, "status.json")
	outputPath := filepath.Join(directory, "generated.go")
	schema := `{"format":1,"command":"test.status","version":1,"request":{},"reply":{}}`
	if err := os.WriteFile(schemaPath, []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if status := run([]string{"schema", "check", schemaPath}, strings.NewReader(""), &stdout, &stderr); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	if status := run([]string{"schema", "generate-go", "cmdschema", outputPath, schemaPath}, strings.NewReader(""), &stdout, &stderr); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	generated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "type TestStatusReply struct") {
		t.Fatalf("generated source = %s", generated)
	}

	outputPath = filepath.Join(directory, "generated.rs")
	if status := run([]string{"schema", "generate-rust", outputPath, schemaPath}, strings.NewReader(""), &stdout, &stderr); status != 0 {
		t.Fatalf("status = %d, stderr = %q", status, stderr.String())
	}
	generated, err = os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "pub struct TestStatusReply") {
		t.Fatalf("generated Rust source = %s", generated)
	}
}

func TestRunReportsFileAndLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records.txt")
	if err := os.WriteFile(path, []byte("request object.status\nbad record\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if status := run([]string{"check", path}, strings.NewReader(""), &stdout, &stderr); status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), path+":2:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsOversizedInput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	input := strings.Repeat("x", 1024*1024) + "\n"
	if status := run([]string{"check"}, strings.NewReader(input), &stdout, &stderr); status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "record exceeds 80 bytes") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunFormatReportsOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	if status := run(
		[]string{"format"}, strings.NewReader("request object.status\n"),
		failingWriter{}, &stderr,
	); status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "stdin:1: writing output: write failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReportsOutputFailure(t *testing.T) {
	schemaPath := filepath.Join(t.TempDir(), "status.json")
	schema := `{"format":1,"command":"test.status","version":1,"request":{},"reply":{}}`
	if err := os.WriteFile(schemaPath, []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if status := run(
		[]string{"schema", "generate-go", "cmdschema", "-", schemaPath},
		strings.NewReader(""), failingWriter{}, &stderr,
	); status != 1 {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(stderr.String(), "write failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunReportsMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if status := run(
		[]string{"check", filepath.Join(t.TempDir(), "missing")},
		strings.NewReader(""), &stdout, &stderr,
	); status != 1 {
		t.Fatalf("status = %d", status)
	}
	if stderr.Len() == 0 {
		t.Fatal("missing file produced no diagnostic")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}
