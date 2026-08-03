package cmdwire

import (
	"fmt"
	"strings"
)

type parsedTail struct {
	resource string
	fields   []Field
}

type lexeme struct {
	token  int
	column int
	text   string
	field  Field
}

type lineLexer struct {
	lexemes []lexeme
	index   int
	record  Record
	err     error
}

func (lexer *lineLexer) Lex(value *cwSymType) int {
	if lexer.index >= len(lexer.lexemes) {
		return 0
	}
	item := lexer.lexemes[lexer.index]
	lexer.index++
	value.text = item.text
	value.field = item.field
	return item.token
}

func (lexer *lineLexer) Error(message string) {
	if lexer.err != nil {
		return
	}
	column := 1
	if lexer.index < len(lexer.lexemes) {
		column = lexer.lexemes[lexer.index].column
	} else if len(lexer.lexemes) > 0 {
		last := lexer.lexemes[len(lexer.lexemes)-1]
		column = last.column + len(last.text)
	}
	lexer.err = syntaxError(column, "%s", message)
}

func setParsedRecord(lexer cwLexer, record Record) {
	lexer.(*lineLexer).record = record
}

func lexLine(line string) ([]lexeme, error) {
	raw, err := splitLine(line)
	if err != nil {
		return nil, err
	}
	kind := raw[0].text
	items := make([]lexeme, 0, len(raw))
	for index, item := range raw {
		lex := lexeme{column: item.column, text: item.text}
		switch index {
		case 0:
			switch item.text {
			case "request":
				lex.token = REQUEST
			case "ok":
				lex.token = OK_TOKEN
			case "err":
				lex.token = ERR
			case "notice":
				lex.token = NOTICE_TOKEN
			case "event":
				lex.token = EVENT_TOKEN
			case "item":
				lex.token = ITEM_TOKEN
			case "chunk":
				lex.token = CHUNK_TOKEN
			default:
				return nil, syntaxError(item.column, "unknown record kind %q", item.text)
			}
		case 1:
			if !validCommand(item.text) {
				return nil, syntaxError(item.column, "invalid command %q", item.text)
			}
			lex.token = COMMAND_TOKEN
		default:
			if strings.Contains(item.text, "=") {
				field, parseErr := parseField(item.text)
				if parseErr != nil {
					return nil, syntaxError(item.column, "%v", parseErr)
				}
				lex.token, lex.field = FIELD_TOKEN, field
				schemaField := kind == "ok" && field.Name == "schema"
				countField := kind == "ok" && field.Name == "count"
				codeField := kind == "err" && field.Name == "code"
				switch {
				case schemaField:
					lex.token = SCHEMA_FIELD
				case countField:
					lex.token = COUNT_FIELD
				case codeField:
					lex.token = CODE_FIELD
				}
			} else {
				if !validResource(item.text) {
					return nil, syntaxError(item.column, "invalid resource path %q", item.text)
				}
				lex.token = PATH_TOKEN
			}
		}
		items = append(items, lex)
	}
	return items, nil
}

type rawLexeme struct {
	text   string
	column int
}

func splitLine(line string) ([]rawLexeme, error) {
	if line == "" {
		return nil, syntaxError(1, "empty record")
	}
	if len(line) > MaxLineBytes {
		return nil, syntaxError(MaxLineBytes+1, "record exceeds %d bytes", MaxLineBytes)
	}
	for index := range len(line) {
		if line[index] < 0x20 || line[index] > 0x7e {
			return nil, syntaxError(index+1, "record contains non-printable ASCII")
		}
	}
	if line[0] == ' ' {
		return nil, syntaxError(1, "leading space")
	}

	var items []rawLexeme
	for start := 0; start < len(line); {
		end := tokenEnd(line, start)
		items = append(items, rawLexeme{text: line[start:end], column: start + 1})
		if end == len(line) {
			break
		}
		if end+1 == len(line) {
			return nil, syntaxError(end+1, "trailing space")
		}
		if line[end+1] == ' ' {
			return nil, syntaxError(end+2, "records require one space between tokens")
		}
		start = end + 1
	}
	return items, nil
}

func tokenEnd(line string, start int) (end int) {
	for end = start; end < len(line) && line[end] != ' '; end++ {
	}
	return
}

func parseField(raw string) (Field, error) {
	equals := strings.IndexByte(raw, '=')
	if equals < 1 {
		return Field{}, fmt.Errorf("expected field name before =")
	}
	name, value := raw[:equals], raw[equals+1:]
	if !validFieldName(name) {
		return Field{}, fmt.Errorf("invalid field name %q", name)
	}
	if !validValue(value) {
		return Field{}, fmt.Errorf("field %q has invalid value", name)
	}
	return Field{Name: name, Value: value}, nil
}
