package cmdwire_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/sartura/cmdwire"
)

type featureState struct {
	record    cmdwire.Record
	parseErr  error
	collector *cmdwire.Collector
	addErr    error
	complete  bool
}

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name:                "cmdwire",
		ScenarioInitializer: initializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			NoColors: true,
			Paths:    []string{"."},
			TestingT: t,
		},
	}
	if status := suite.Run(); status != 0 {
		t.Fatalf("feature suite returned status %d", status)
	}
}

func initializeScenario(scenario *godog.ScenarioContext) {
	state := &featureState{}
	scenario.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		*state = featureState{}
		return ctx, nil
	})

	scenario.Step(`^I parse this record:$`, state.parseRecord)
	scenario.Step(`^parsing succeeds$`, state.parsingSucceeds)
	scenario.Step(`^parsing fails$`, state.parsingFails)
	scenario.Step(`^the record kind is "([^"]*)"$`, state.recordKindIs)
	scenario.Step(`^the command is "([^"]*)"$`, state.commandIs)
	scenario.Step(`^field "([^"]*)" is "([^"]*)"$`, state.fieldIs)
	scenario.Step(`^the canonical record is "([^"]*)"$`, state.canonicalRecordIs)
	scenario.Step(`^I collect replies for "([^"]*)"$`, state.collectRepliesFor)
	scenario.Step(`^these lines arrive:$`, state.linesArrive)
	scenario.Step(`^line collection succeeds$`, state.collectionSucceeds)
	scenario.Step(`^the reply is complete$`, state.replyIsComplete)
	scenario.Step(`^the reply schema is (\d+)$`, state.replySchemaIs)
	scenario.Step(`^the reply contains (\d+) data records$`, state.replyDataCountIs)
	scenario.Step(`^reply validation fails with "([^"]*)"$`, state.validationFailsWith)
	scenario.Step(`^the remote error code is "([^"]*)"$`, state.remoteCodeIs)
}

func (state *featureState) parseRecord(text *godog.DocString) {
	state.record, state.parseErr = cmdwire.ParseLine(text.Content)
}

func (state *featureState) parsingSucceeds() error {
	if state.parseErr != nil {
		return fmt.Errorf("expected parsing to succeed: %w", state.parseErr)
	}
	return nil
}

func (state *featureState) parsingFails() error {
	if state.parseErr == nil {
		return fmt.Errorf("expected parsing to fail")
	}
	return nil
}

func (state *featureState) recordKindIs(kind string) error {
	if string(state.record.Kind) != kind {
		return fmt.Errorf("record kind is %q, want %q", state.record.Kind, kind)
	}
	return nil
}

func (state *featureState) commandIs(command string) error {
	if state.record.Command != command {
		return fmt.Errorf("command is %q, want %q", state.record.Command, command)
	}
	return nil
}

func (state *featureState) fieldIs(name, want string) error {
	value, ok := state.record.Field(name)
	if !ok {
		return fmt.Errorf("field %q is absent", name)
	}
	if value != want {
		return fmt.Errorf("field %q is %q, want %q", name, value, want)
	}
	return nil
}

func (state *featureState) canonicalRecordIs(want string) error {
	line, err := cmdwire.Format(state.record)
	if err != nil {
		return err
	}
	if line != want {
		return fmt.Errorf("canonical record is %q, want %q", line, want)
	}
	return nil
}

func (state *featureState) collectRepliesFor(command string) error {
	collector, err := cmdwire.NewCollector(command)
	state.collector = collector
	return err
}

func (state *featureState) linesArrive(text *godog.DocString) {
	for _, line := range strings.Split(strings.Trim(text.Content, "\n"), "\n") {
		_, complete, err := state.collector.AddLine(line)
		if err != nil {
			state.addErr = err
			return
		}
		state.complete = complete
	}
}

func (state *featureState) collectionSucceeds() error {
	return state.addErr
}

func (state *featureState) replyIsComplete() error {
	if !state.complete {
		return fmt.Errorf("reply is incomplete")
	}
	return nil
}

func (state *featureState) replySchemaIs(want uint64) error {
	report, err := state.collector.Result()
	if err != nil {
		return err
	}
	if report.Schema != want {
		return fmt.Errorf("reply schema is %d, want %d", report.Schema, want)
	}
	return nil
}

func (state *featureState) replyDataCountIs(want int) error {
	report, err := state.collector.Result()
	if err != nil {
		return err
	}
	if len(report.Data) != want {
		return fmt.Errorf("reply has %d data records, want %d", len(report.Data), want)
	}
	return nil
}

func (state *featureState) validationFailsWith(want string) error {
	_, err := state.collector.Result()
	if err == nil {
		return fmt.Errorf("expected reply validation to fail")
	}
	if !strings.Contains(err.Error(), want) {
		return fmt.Errorf("validation error %q does not contain %q", err, want)
	}
	return nil
}

func (state *featureState) remoteError() (*cmdwire.RemoteError, error) {
	_, err := state.collector.Result()
	var remote *cmdwire.RemoteError
	if !errors.As(err, &remote) {
		return nil, fmt.Errorf("result error is %T, want *cmdwire.RemoteError", err)
	}
	return remote, nil
}

func (state *featureState) remoteCodeIs(want string) error {
	remote, err := state.remoteError()
	if err != nil {
		return err
	}
	if remote.Code != want {
		return fmt.Errorf("remote code is %q, want %q", remote.Code, want)
	}
	return nil
}
