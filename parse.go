package cmdwire

import (
	"fmt"
	"strings"
)

// ParseLine parses one cmdwire record. The input may omit its line ending
// or end in LF or CRLF.
func ParseLine(input string) (Record, error) {
	line, err := stripLineEnding(input)
	if err != nil {
		return Record{}, err
	}
	lexemes, err := lexLine(line)
	if err != nil {
		return Record{}, err
	}
	lexer := &lineLexer{lexemes: lexemes}
	if cwParse(lexer) != 0 {
		if lexer.err != nil {
			return Record{}, lexer.err
		}
		return Record{}, syntaxError(1, "invalid record")
	}
	if lexer.err != nil {
		return Record{}, lexer.err
	}
	if err := validateRecord(lexer.record); err != nil {
		return Record{}, syntaxError(1, "%v", err)
	}
	return lexer.record, nil
}

func stripLineEnding(input string) (string, error) {
	line := input
	switch {
	case strings.HasSuffix(line, "\r\n"):
		line = line[:len(line)-2]
	case strings.HasSuffix(line, "\n"):
		line = line[:len(line)-1]
	}
	if strings.ContainsAny(line, "\r\n") {
		return "", syntaxError(1, "record contains an embedded line ending")
	}
	return line, nil
}

func wrapValidation(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("cmdwire: %w", err)
}
