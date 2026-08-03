package cmdwire

import (
	"fmt"
	"strconv"
	"strings"
)

func validateRecord(record Record) error {
	if !validCommand(record.Command) {
		return fmt.Errorf("invalid command %q", record.Command)
	}

	seen := make(map[string]bool, len(record.Fields))
	for _, field := range record.Fields {
		if !validFieldName(field.Name) {
			return fmt.Errorf("invalid field name %q", field.Name)
		}
		if !validValue(field.Value) {
			return fmt.Errorf("field %q has invalid value", field.Name)
		}
		if seen[field.Name] {
			return fmt.Errorf("duplicate field %q", field.Name)
		}
		seen[field.Name] = true
	}

	resourceAllowed := false
	switch record.Kind {
	case Request:
		resourceAllowed = true
	case OK:
		if len(record.Fields) < 2 {
			return fmt.Errorf("ok record requires schema and count")
		}
		if record.Fields[0].Name != "schema" {
			return fmt.Errorf("ok record's first field must be schema")
		}
		if !validDecimal(record.Fields[0].Value, true) {
			return fmt.Errorf("invalid schema %q", record.Fields[0].Value)
		}
		if record.Fields[1].Name != "count" {
			return fmt.Errorf("ok record's second field must be count")
		}
		if !validDecimal(record.Fields[1].Value, false) {
			return fmt.Errorf("invalid count %q", record.Fields[1].Value)
		}
	case Error:
		if len(record.Fields) < 1 {
			return fmt.Errorf("error record requires code")
		}
		if record.Fields[0].Name != "code" {
			return fmt.Errorf("error record's first field must be code")
		}
		if !validErrorCode(record.Fields[0].Value) {
			return fmt.Errorf("invalid error code %q", record.Fields[0].Value)
		}
	case Notice, Event, Chunk:
		resourceAllowed = true
		if len(record.Fields) == 0 {
			return fmt.Errorf("%s record requires at least one field", record.Kind)
		}
	case Item:
		resourceAllowed = true
	default:
		return fmt.Errorf("invalid record kind %q", record.Kind)
	}

	if record.Resource != "" {
		if !resourceAllowed {
			return fmt.Errorf("%s record cannot contain a resource path", record.Kind)
		}
		if !validResource(record.Resource) {
			return fmt.Errorf("invalid resource path %q", record.Resource)
		}
	}

	return nil
}

func validCommand(command string) bool {
	if command == "" {
		return false
	}
	for _, segment := range strings.Split(command, ".") {
		if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
			return false
		}
		for _, char := range segment[1:] {
			if !lower(char) && !digit(char) && char != '_' && char != '-' {
				return false
			}
		}
	}
	return true
}

func validResource(resource string) bool {
	if resource == "" {
		return false
	}
	for _, segment := range strings.Split(resource, "/") {
		parts := strings.Split(segment, ".")
		if len(parts) > 2 {
			return false
		}
		for _, part := range parts {
			if part == "" || !alphaNumeric(rune(part[0])) ||
				!alphaNumeric(rune(part[len(part)-1])) {
				return false
			}
			for _, char := range part {
				if !lower(char) && !upper(char) && !digit(char) &&
					char != '_' && char != ':' && char != '-' {
					return false
				}
			}
		}
	}
	return true
}

func validValue(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range []byte(value) {
		if char <= ' ' || char > '~' || char == '"' || char == '\\' {
			return false
		}
	}
	return true
}

func validDecimal(value string, positive bool) bool {
	if value == "0" {
		return !positive
	}
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, char := range value[1:] {
		if !digit(char) {
			return false
		}
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func validFieldName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, char := range name[1:] {
		if !lower(char) && !digit(char) && char != '_' {
			return false
		}
	}
	return true
}

func validErrorCode(code string) bool {
	if code == "" || code[0] < 'A' || code[0] > 'Z' {
		return false
	}
	for _, char := range code[1:] {
		if !upper(char) && !digit(char) && char != '_' {
			return false
		}
	}
	return true
}

func lower(char rune) bool { return char >= 'a' && char <= 'z' }
func upper(char rune) bool { return char >= 'A' && char <= 'Z' }
func digit(char rune) bool { return char >= '0' && char <= '9' }
func alphaNumeric(char rune) bool {
	return lower(char) || upper(char) || digit(char)
}

func requiredUint(fields []Field, name string) (uint64, error) {
	var raw string
	found := false
	for _, field := range fields {
		if field.Name == name {
			raw, found = field.Value, true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("missing %s", name)
	}
	if !validDecimal(raw, name == "schema") {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q", name, raw)
	}
	return value, nil
}
