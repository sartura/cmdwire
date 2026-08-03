package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sartura/cmdwire"
)

type conformanceCorpus struct {
	Valid   []corpusCase `json:"valid"`
	Invalid []corpusCase `json:"invalid"`
}

type corpusCase struct {
	Line string `json:"line"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) < 3 {
		fmt.Fprintln(stderr, "usage: differential ORACLE CONFORMANCE CORPUS_DIR...")
		return 2
	}
	inputs, err := loadInputs(args[1], args[2:])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	results, err := runOracle(args[0], inputs)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for index, input := range inputs {
		want := describe(input)
		if results[index] != want {
			fmt.Fprintf(stderr, "parser mismatch for %q: Go %q, Rust %q\n", input, want, results[index])
			return 1
		}
	}
	fmt.Fprintf(stdout, "Go and Rust agree on %d parser inputs\n", len(inputs))
	return 0
}

func loadInputs(conformancePath string, corpusPaths []string) ([]string, error) {
	data, err := os.ReadFile(conformancePath)
	if err != nil {
		return nil, fmt.Errorf("read conformance corpus: %w", err)
	}
	var conformance conformanceCorpus
	if err := json.Unmarshal(data, &conformance); err != nil {
		return nil, fmt.Errorf("decode conformance corpus: %w", err)
	}
	unique := make(map[string]bool, 3*(len(conformance.Valid)+len(conformance.Invalid)))
	for _, test := range append(conformance.Valid, conformance.Invalid...) {
		unique[test.Line] = true
		unique[test.Line+"\n"] = true
		unique[test.Line+"\r\n"] = true
	}
	for _, path := range corpusPaths {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("read fuzz corpus %s: %w", path, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(path, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("read fuzz corpus entry %s: %w", entry.Name(), err)
			}
			input, err := decodeCorpusEntry(data)
			if err != nil {
				return nil, fmt.Errorf("decode fuzz corpus entry %s: %w", entry.Name(), err)
			}
			unique[input] = true
		}
	}
	inputs := make([]string, 0, len(unique))
	for input := range unique {
		inputs = append(inputs, input)
	}
	sort.Strings(inputs)
	return inputs, nil
}

func decodeCorpusEntry(data []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "go test fuzz v1" {
		return "", fmt.Errorf("unsupported corpus format")
	}
	const prefix = "string("
	if !strings.HasPrefix(lines[1], prefix) || !strings.HasSuffix(lines[1], ")") {
		return "", fmt.Errorf("entry does not contain one string")
	}
	quoted := strings.TrimSuffix(strings.TrimPrefix(lines[1], prefix), ")")
	input, err := strconv.Unquote(quoted)
	if err != nil {
		return "", fmt.Errorf("decode string: %w", err)
	}
	return input, nil
}

func runOracle(path string, inputs []string) ([]string, error) {
	var stdin strings.Builder
	for _, input := range inputs {
		stdin.WriteString(hex.EncodeToString([]byte(input)))
		stdin.WriteByte('\n')
	}
	command := exec.Command(path)
	command.Stdin = strings.NewReader(stdin.String())
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("run Rust parser oracle: %w\n%s", err, output)
	}
	text := strings.TrimSuffix(string(output), "\n")
	results := strings.Split(text, "\n")
	if len(inputs) == 0 {
		results = nil
	}
	if len(results) != len(inputs) {
		return nil, fmt.Errorf("Rust parser oracle returned %d results for %d inputs", len(results), len(inputs))
	}
	return results, nil
}

func describe(input string) string {
	record, err := cmdwire.ParseLine(input)
	if err != nil {
		return "reject"
	}
	parts := []string{"accept", string(record.Kind), record.Command, record.Resource}
	for _, field := range record.Fields {
		parts = append(parts, field.Name+"="+field.Value)
	}
	return strings.Join(parts, "\t")
}
