// Package cmdwire parses and formats the cmdwire line protocol.
package cmdwire

import "fmt"

// MaxLineBytes is the maximum record width, excluding the line ending.
const MaxLineBytes = 80

// Kind identifies a cmdwire record.
type Kind string

const (
	Request Kind = "request"
	OK      Kind = "ok"
	Error   Kind = "err"
	Notice  Kind = "notice"
	Event   Kind = "event"
	Item    Kind = "item"
	Chunk   Kind = "chunk"
)

// Field is one ordered key-value pair.
type Field struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Record is one parsed cmdwire line.
type Record struct {
	Kind     Kind    `json:"kind"`
	Command  string  `json:"command"`
	Resource string  `json:"resource,omitempty"`
	Fields   []Field `json:"fields,omitempty"`
}

// Field returns the unique field with name. Parsed records always have unique
// names, but constructed records are checked defensively.
func (record Record) Field(name string) (value string, ok bool) {
	for _, field := range record.Fields {
		if field.Name != name {
			continue
		}
		if ok {
			return "", false
		}
		value, ok = field.Value, true
	}
	return
}

// SyntaxError reports a one-based byte column in a physical record.
type SyntaxError struct {
	Column  int
	Message string
}

func (err *SyntaxError) Error() string {
	if err.Column <= 0 {
		return "cmdwire: " + err.Message
	}
	return fmt.Sprintf("cmdwire: column %d: %s", err.Column, err.Message)
}

func syntaxError(column int, format string, args ...any) error {
	return &SyntaxError{Column: column, Message: fmt.Sprintf(format, args...)}
}

func cloneRecord(record Record) Record {
	record.Fields = append([]Field(nil), record.Fields...)
	return record
}
