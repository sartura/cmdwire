package cmdwire

import (
	"fmt"
	"strings"
)

// Format returns the canonical wire representation of record without a line
// ending.
func Format(record Record) (string, error) {
	if err := validateRecord(record); err != nil {
		return "", wrapValidation(err)
	}

	var output strings.Builder
	output.WriteString(string(record.Kind))
	output.WriteByte(' ')
	output.WriteString(record.Command)
	if record.Resource != "" {
		output.WriteByte(' ')
		output.WriteString(record.Resource)
	}
	for _, field := range record.Fields {
		output.WriteByte(' ')
		output.WriteString(field.Name)
		output.WriteByte('=')
		output.WriteString(field.Value)
	}
	if output.Len() > MaxLineBytes {
		return "", fmt.Errorf(
			"cmdwire: formatted record is %d bytes, maximum is %d",
			output.Len(), MaxLineBytes,
		)
	}
	return output.String(), nil
}
