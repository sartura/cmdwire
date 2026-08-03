package cmdwire

import (
	"bufio"
	"errors"
	"io"
)

// Decoder reads cmdwire records from a line-oriented stream.
type Decoder struct {
	reader *bufio.Reader
}

// NewDecoder returns a bounded line decoder for input.
func NewDecoder(input io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(input, MaxLineBytes+2)}
}

// Decode reads and parses the next record.
func (decoder *Decoder) Decode() (Record, error) {
	line, prefix, err := decoder.reader.ReadLine()
	if errors.Is(err, io.EOF) && len(line) == 0 {
		return Record{}, io.EOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return Record{}, err
	}
	if prefix || len(line) > MaxLineBytes {
		for prefix {
			_, prefix, err = decoder.reader.ReadLine()
			if err != nil && !errors.Is(err, io.EOF) {
				return Record{}, err
			}
		}
		return Record{}, syntaxError(MaxLineBytes+1, "record exceeds %d bytes", MaxLineBytes)
	}
	return ParseLine(string(line))
}
