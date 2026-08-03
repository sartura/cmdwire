package cmdwire

import (
	"errors"
	"strings"
	"testing"
)

func mustCollector(t *testing.T, command string) *Collector {
	t.Helper()
	collector, err := NewCollector(command)
	if err != nil {
		t.Fatal(err)
	}
	return collector
}

func TestCollectorExtractsInterleavedReply(t *testing.T) {
	collector := mustCollector(t, "object.status")
	lines := []string{
		"ordinary console output",
		"event other.watch state=waiting",
		"notice object.status lifecycle state=ready",
		"item object.status alpha state=ready",
		"debug: still running",
		"item object.status beta state=idle",
		"ok object.status schema=1 count=2",
	}
	for index, line := range lines {
		matched, complete, err := collector.AddLine(line)
		if err != nil {
			t.Fatalf("line %d: %v", index, err)
		}
		if index < len(lines)-1 && complete {
			t.Fatalf("line %d completed reply early", index)
		}
		if index == len(lines)-1 && (!matched || !complete) {
			t.Fatalf("terminal = matched %v, complete %v", matched, complete)
		}
	}
	report, err := collector.Result()
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != 1 || len(report.Data) != 2 {
		t.Fatalf("report = %#v", report)
	}
}

func TestCollectorIgnoresNoticeForObservedCommand(t *testing.T) {
	collector := mustCollector(t, "object.status")
	matched, complete, err := collector.AddLine("notice object.status lifecycle state=ready")
	if matched || complete || err != nil {
		t.Fatalf("notice = matched %v, complete %v, error %v", matched, complete, err)
	}
}

func TestCollectorAddRejectsNotice(t *testing.T) {
	collector := mustCollector(t, "object.status")
	err := collector.Add(Record{
		Kind: Notice, Command: "object.status", Resource: "lifecycle",
		Fields: []Field{{Name: "state", Value: "ready"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not a reply record") {
		t.Fatalf("notice error = %v", err)
	}
}

func TestCollectorRejectsMalformedMatchingLine(t *testing.T) {
	collector := mustCollector(t, "object.status")
	matched, _, err := collector.AddLine("item\tobject.status state=ready")
	if !matched || err == nil {
		t.Fatalf("matched = %v, error = %v", matched, err)
	}
	matched, _, err = collector.AddLine("item other.status")
	if matched || err != nil {
		t.Fatalf("unrelated line: matched = %v, error = %v", matched, err)
	}
}

func TestCollectorRejectsMalformedTerminal(t *testing.T) {
	for _, line := range []string{
		"ok object.status count=1",
		"ok object.status schema=1",
		"ok object.status schema=0 count=1",
		"ok object.status schema=1 count=+1",
	} {
		collector := mustCollector(t, "object.status")
		matched, _, err := collector.AddLine(line)
		if !matched || err == nil {
			t.Fatalf("AddLine(%q) = matched %v, error %v", line, matched, err)
		}
	}
}

func TestCollectorValidatesTrailerCount(t *testing.T) {
	collector := mustCollector(t, "object.status")
	if _, _, err := collector.AddLine("item object.status state=ready"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := collector.AddLine("ok object.status schema=1 count=2"); err != nil {
		t.Fatal(err)
	}
	if report, err := collector.Result(); err == nil {
		t.Fatalf("Result succeeded: %#v", report)
	}
}

func TestCollectorAcceptsEmptyReplies(t *testing.T) {
	for _, line := range []string{
		"ok object.status schema=1 count=0 state=ready",
		"ok object.status schema=1 count=0",
	} {
		collector := mustCollector(t, "object.status")
		if _, complete, err := collector.AddLine(line); err != nil || !complete {
			t.Fatalf("AddLine(%q) = complete %v, error %v", line, complete, err)
		}
		if _, err := collector.Result(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollectorReturnsRemoteErrorAndDiscardsData(t *testing.T) {
	collector := mustCollector(t, "object.status")
	_, _, _ = collector.AddLine("item object.status state=partial")
	_, complete, err := collector.AddLine(
		"err object.status code=IO_ERROR retry=false",
	)
	if err != nil || !complete {
		t.Fatalf("complete = %v, error = %v", complete, err)
	}
	_, err = collector.Result()
	var remote *RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("error = %T %v", err, err)
	}
	if remote.Code != "IO_ERROR" || len(remote.Fields) != 1 {
		t.Fatalf("remote error = %#v", remote)
	}
	if len(collector.data) != 0 {
		t.Fatal("data survived terminal error")
	}
}

func TestCollectorBoundsDataRecords(t *testing.T) {
	if _, err := NewCollectorWithLimit("object.status", 0); err == nil {
		t.Fatal("zero data-record limit accepted")
	}
	collector, err := NewCollectorWithLimit("object.status", 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"alpha", "beta"} {
		if err := collector.Add(Record{
			Kind: Item, Command: "object.status", Resource: resource,
		}); err != nil {
			t.Fatal(err)
		}
	}
	err = collector.Add(Record{Kind: Item, Command: "object.status", Resource: "gamma"})
	if !errors.Is(err, ErrDataLimit) {
		t.Fatalf("limit error = %v", err)
	}
	if _, err := collector.Result(); !errors.Is(err, ErrDataLimit) {
		t.Fatalf("result error = %v", err)
	}
	if len(collector.data) != 0 {
		t.Fatal("collector retained data after reaching its limit")
	}
}

func TestRemoteErrorEscapesInvalidParts(t *testing.T) {
	err := (&RemoteError{Command: "object\nstatus", Code: "BAD\nCODE"}).Error()
	if strings.Contains(err, "\n") {
		t.Fatalf("error contains a line break: %q", err)
	}
	for _, escaped := range []string{`object\nstatus`, `BAD\nCODE`} {
		if !strings.Contains(err, escaped) {
			t.Fatalf("error %q does not contain %q", err, escaped)
		}
	}
}

func TestCollectorOwnsAddedRecords(t *testing.T) {
	collector := mustCollector(t, "object.status")
	fields := []Field{{Name: "state", Value: "ready"}}
	if err := collector.Add(Record{Kind: Item, Command: "object.status", Fields: fields}); err != nil {
		t.Fatal(err)
	}
	fields[0].Value = "mutated"
	if err := collector.Add(Record{
		Kind: OK, Command: "object.status",
		Fields: []Field{{Name: "schema", Value: "1"}, {Name: "count", Value: "1"}},
	}); err != nil {
		t.Fatal(err)
	}
	report, err := collector.Result()
	if err != nil {
		t.Fatal(err)
	}
	if report.Data[0].Fields[0].Value != "ready" {
		t.Fatalf("stored value = %q", report.Data[0].Fields[0].Value)
	}
}

func TestCollectorAddLineAfterCompletion(t *testing.T) {
	collector := mustCollector(t, "object.status")
	if _, complete, err := collector.AddLine("ok object.status schema=1 count=0"); err != nil || !complete {
		t.Fatalf("terminal: complete %v, error %v", complete, err)
	}
	matched, complete, err := collector.AddLine("item object.status state=late")
	if !errors.Is(err, ErrComplete) || !matched || !complete {
		t.Fatalf("late record: matched %v, complete %v, error %v", matched, complete, err)
	}
}

func TestCollectorStateErrors(t *testing.T) {
	if _, err := NewCollector("Bad"); err == nil {
		t.Fatal("invalid command accepted")
	}
	collector := mustCollector(t, "object.status")
	if _, err := collector.Result(); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("incomplete error = %v", err)
	}
	if err := collector.Add(Record{Kind: Request, Command: "object.status"}); err == nil {
		t.Fatal("request record accepted as reply")
	}
	if err := collector.Add(Record{
		Kind: Item, Command: "object.status",
		Fields: []Field{{Name: "value", Value: strings.Repeat("x", MaxLineBytes)}},
	}); err == nil {
		t.Fatal("oversized record accepted")
	}
	otherTerminal := Record{Kind: OK, Command: "other.status", Fields: []Field{
		{Name: "schema", Value: "1"}, {Name: "count", Value: "0"},
	}}
	if err := collector.Add(otherTerminal); err == nil {
		t.Fatal("other command accepted")
	}
	terminal := Record{Kind: OK, Command: "object.status", Fields: []Field{
		{Name: "schema", Value: "1"}, {Name: "count", Value: "0"},
	}}
	if err := collector.Add(terminal); err != nil {
		t.Fatal(err)
	}
	if err := collector.Add(terminal); !errors.Is(err, ErrComplete) {
		t.Fatalf("second terminal error = %v", err)
	}
}

func TestLikelyReplyDoesNotClaimUnrelatedMalformedOutput(t *testing.T) {
	collector := mustCollector(t, "object.status")
	for _, line := range []string{
		"not protocol",
		"item other.status state=ready state=again",
		strings.Repeat("x", MaxLineBytes+1),
	} {
		matched, _, err := collector.AddLine(line)
		if matched || err != nil {
			t.Fatalf("AddLine(%q) = matched %v, error %v", line, matched, err)
		}
	}
}
