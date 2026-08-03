package cmdwire

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultMaxDataRecords bounds data buffered by NewCollector.
const DefaultMaxDataRecords = 1024

var (
	// ErrIncomplete means that no matching terminal record has arrived.
	ErrIncomplete = errors.New("cmdwire: reply is incomplete")
	// ErrComplete means that a matching record arrived after the terminal.
	ErrComplete = errors.New("cmdwire: reply is already complete")
	// ErrDataLimit means that a reply exceeded its collector's record limit.
	ErrDataLimit = errors.New("cmdwire: reply data record limit exceeded")
)

// Report is one successful command reply.
type Report struct {
	Command  string
	Schema   uint64
	Data     []Record
	Terminal []Field
}

// RemoteError is a terminal err record. Data emitted before it is discarded.
type RemoteError struct {
	Command string
	Code    string
	Fields  []Field
}

func (err *RemoteError) Error() string {
	command := err.Command
	if !validCommand(command) {
		command = strconv.QuoteToASCII(command)
	}
	code := err.Code
	if !validErrorCode(code) {
		code = strconv.QuoteToASCII(code)
	}
	return fmt.Sprintf("cmdwire: %s failed with %s", command, code)
}

// Collector extracts one command-scoped reply from interleaved console lines.
type Collector struct {
	command        string
	maxDataRecords int
	data           []Record
	terminal       *Record
	failure        error
}

// NewCollector starts bounded collection for command.
func NewCollector(command string) (*Collector, error) {
	return NewCollectorWithLimit(command, DefaultMaxDataRecords)
}

// NewCollectorWithLimit starts collection with a positive data-record limit.
func NewCollectorWithLimit(command string, maxDataRecords int) (*Collector, error) {
	if !validCommand(command) {
		return nil, fmt.Errorf("cmdwire: invalid command %q", command)
	}
	if maxDataRecords <= 0 {
		return nil, fmt.Errorf("cmdwire: invalid data record limit %d", maxDataRecords)
	}
	return &Collector{command: command, maxDataRecords: maxDataRecords}, nil
}

// AddLine parses a physical line. Unrelated and other-command lines are
// ignored. matched reports whether the line belongs to the observed command.
func (collector *Collector) AddLine(line string) (matched bool, complete bool, err error) {
	if collector.failure != nil {
		return false, false, collector.failure
	}
	record, err := ParseLine(line)
	if err != nil {
		if likelyReplyFor(line, collector.command) {
			return true, false, err
		}
		return false, collector.terminal != nil, nil
	}
	if record.Command != collector.command || !replyKind(record.Kind) {
		return false, collector.terminal != nil, nil
	}
	if err := collector.Add(record); err != nil {
		return true, collector.terminal != nil, err
	}
	return true, collector.terminal != nil, nil
}

// Add adds an already parsed reply record for the observed command. Unlike
// AddLine, it rejects notices and other non-reply records.
func (collector *Collector) Add(record Record) error {
	if collector.failure != nil {
		return collector.failure
	}
	if _, err := Format(record); err != nil {
		return err
	}
	if record.Command != collector.command {
		return fmt.Errorf(
			"cmdwire: record command is %s, collector observes %s",
			record.Command, collector.command,
		)
	}
	if !replyKind(record.Kind) {
		return fmt.Errorf("cmdwire: %s is not a reply record", record.Kind)
	}
	if collector.terminal != nil {
		return ErrComplete
	}

	record = cloneRecord(record)
	switch record.Kind {
	case Event, Item, Chunk:
		if len(collector.data) == collector.maxDataRecords {
			collector.data = nil
			collector.failure = fmt.Errorf(
				"%w: maximum is %d", ErrDataLimit, collector.maxDataRecords,
			)
			return collector.failure
		}
		collector.data = append(collector.data, record)
	case OK:
		terminal := record
		collector.terminal = &terminal
	case Error:
		collector.data = nil
		terminal := record
		collector.terminal = &terminal
	}
	return nil
}

// Result validates and returns the completed reply.
func (collector *Collector) Result() (Report, error) {
	if collector.failure != nil {
		return Report{}, collector.failure
	}
	if collector.terminal == nil {
		return Report{}, ErrIncomplete
	}
	terminal := *collector.terminal
	if terminal.Kind == Error {
		return Report{}, &RemoteError{
			Command: terminal.Command,
			Code:    terminal.Fields[0].Value,
			Fields:  append([]Field(nil), terminal.Fields[1:]...),
		}
	}

	schema, err := requiredUint(terminal.Fields, "schema")
	if err != nil {
		return Report{}, fmt.Errorf("cmdwire: validating %s trailer: %w", collector.command, err)
	}
	count, err := requiredUint(terminal.Fields, "count")
	if err != nil {
		return Report{}, fmt.Errorf("cmdwire: validating %s trailer: %w", collector.command, err)
	}
	if count != uint64(len(collector.data)) {
		return Report{}, fmt.Errorf(
			"cmdwire: %s declares %d data records, collected %d",
			collector.command, count, len(collector.data),
		)
	}

	data := make([]Record, len(collector.data))
	for index, record := range collector.data {
		data[index] = cloneRecord(record)
	}
	return Report{
		Command:  collector.command,
		Schema:   schema,
		Data:     data,
		Terminal: append([]Field(nil), terminal.Fields...),
	}, nil
}

func replyKind(kind Kind) bool {
	switch kind {
	case OK, Error, Event, Item, Chunk:
		return true
	default:
		return false
	}
}

func likelyReplyFor(line, command string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[1] != command {
		return false
	}
	switch Kind(fields[0]) {
	case OK, Error, Event, Item, Chunk:
		return true
	default:
		return false
	}
}
