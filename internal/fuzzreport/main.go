package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sartura/cmdwire"
)

type report struct {
	total             int
	accepted          int
	kinds             map[string]int
	rejections        map[string]int
	canonicalExamples []string
	longest           string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: fuzzreport CORPUS_DIR")
		os.Exit(2)
	}
	result, err := analyze(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	result.print(os.Args[1])
}

func analyze(path string) (*report, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus %s: %w", path, err)
	}
	result := &report{
		kinds:      make(map[string]int),
		rejections: make(map[string]int),
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := filepath.Join(path, entry.Name())
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read corpus entry %s: %w", name, err)
		}
		input, err := decodeCorpusEntry(data)
		if err != nil {
			return nil, fmt.Errorf("decode corpus entry %s: %w", name, err)
		}
		result.total++
		record, err := cmdwire.ParseLine(input)
		if err != nil {
			result.rejections[classifyError(err)]++
			continue
		}
		result.accepted++
		result.kinds[string(record.Kind)]++
		if len(input) > len(result.longest) {
			result.longest = input
		}
		canonical, err := cmdwire.Format(record)
		if err != nil {
			return nil, fmt.Errorf("format accepted entry %s: %w", name, err)
		}
		if canonical != input && len(result.canonicalExamples) < 5 {
			result.canonicalExamples = append(
				result.canonicalExamples,
				fmt.Sprintf("%q -> %q", input, canonical),
			)
		}
	}
	return result, nil
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

func classifyError(err error) string {
	message := err.Error()
	classes := []struct {
		contains string
		name     string
	}{
		{"maximum is 80", "record exceeds 80 bytes"},
		{"non-printable ASCII", "non-printable ASCII"},
		{"embedded line ending", "embedded line ending"},
		{"has invalid value", "invalid field value"},
		{"duplicate field", "duplicate field"},
		{"invalid resource path", "invalid resource path"},
		{"invalid command", "invalid command"},
		{"invalid field name", "invalid field name"},
		{"invalid error code", "invalid error code"},
		{"ok record", "invalid success record"},
		{"error record", "invalid error record"},
		{"unknown record kind", "unknown record kind"},
		{"requires one space", "invalid spacing"},
		{"leading space", "leading space"},
		{"trailing space", "trailing space"},
	}
	for _, class := range classes {
		if strings.Contains(message, class.contains) {
			return class.name
		}
	}
	return "other syntax error"
}

func (result *report) print(path string) {
	fmt.Printf("Corpus: %s\n", path)
	fmt.Printf("Entries: %d\n", result.total)
	fmt.Printf("Accepted: %d\n", result.accepted)
	fmt.Printf("Rejected: %d\n", result.total-result.accepted)
	if result.longest != "" {
		fmt.Printf("Longest accepted: %d bytes %q\n", len(result.longest), result.longest)
	}
	printCounts("Accepted kinds", result.kinds)
	printCounts("Rejection classes", result.rejections)
	if len(result.canonicalExamples) != 0 {
		fmt.Println("Canonicalized examples:")
		for _, example := range result.canonicalExamples {
			fmt.Printf("  %s\n", example)
		}
	}
}

func printCounts(title string, counts map[string]int) {
	type count struct {
		name  string
		value int
	}
	ordered := make([]count, 0, len(counts))
	for name, value := range counts {
		ordered = append(ordered, count{name: name, value: value})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].value != ordered[j].value {
			return ordered[i].value > ordered[j].value
		}
		return ordered[i].name < ordered[j].name
	})
	fmt.Printf("%s:\n", title)
	for _, item := range ordered {
		fmt.Printf("  %4d %s\n", item.value, item.name)
	}
}
