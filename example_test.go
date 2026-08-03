package cmdwire_test

import (
	"fmt"

	"github.com/sartura/cmdwire"
)

func ExampleParseLine() {
	record, err := cmdwire.ParseLine(
		"item object.status alpha state=ready",
	)
	if err != nil {
		panic(err)
	}
	state, _ := record.Field("state")
	fmt.Println(record.Kind, record.Command, record.Resource, state)
	// Output: item object.status alpha ready
}

func ExampleCollector() {
	collector, err := cmdwire.NewCollector("object.status")
	if err != nil {
		panic(err)
	}
	for _, line := range []string{
		"unrelated console output",
		"item object.status alpha state=ready",
		"ok object.status schema=1 count=1",
	} {
		_, complete, err := collector.AddLine(line)
		if err != nil {
			panic(err)
		}
		if complete {
			break
		}
	}
	report, err := collector.Result()
	if err != nil {
		panic(err)
	}
	fmt.Println(report.Command, report.Schema, len(report.Data))
	// Output: object.status 1 1
}
