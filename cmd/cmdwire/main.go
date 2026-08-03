package main

import (
	"fmt"
	"io"
	"os"

	"github.com/sartura/cmdwire"
	"github.com/sartura/cmdwire/internal/schemagen"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "schema" {
		return runSchema(args[1:], stdout, stderr)
	}
	if len(args) < 1 || (args[0] != "check" && args[0] != "format") {
		usage(stderr)
		return 2
	}

	paths := args[1:]
	if len(paths) == 0 {
		if err := process(args[0], "stdin", stdin, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}

	failed := false
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		err = process(args[0], path, file, stdout)
		_ = file.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func runSchema(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 || (args[0] != "check" && args[0] != "generate-go" && args[0] != "generate-rust") {
		usage(stderr)
		return 2
	}

	action := args[0]
	var packageName, outputPath string
	paths := args[1:]
	switch action {
	case "generate-go":
		if len(args) < 4 {
			usage(stderr)
			return 2
		}
		packageName, outputPath, paths = args[1], args[2], args[3:]
	case "generate-rust":
		if len(args) < 3 {
			usage(stderr)
			return 2
		}
		outputPath, paths = args[1], args[2:]
	}

	schemas := make([]cmdwire.Schema, 0, len(paths))
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, err)
			return 1
		}
		schema, decodeErr := cmdwire.DecodeSchema(file)
		closeErr := file.Close()
		if decodeErr != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, decodeErr)
			return 1
		}
		if closeErr != nil {
			fmt.Fprintf(stderr, "%s: %v\n", path, closeErr)
			return 1
		}
		schemas = append(schemas, schema)
	}
	if action == "check" {
		return 0
	}

	var generated []byte
	var err error
	if action == "generate-go" {
		generated, err = schemagen.Go(packageName, schemas)
	} else {
		generated, err = schemagen.Rust(schemas)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if outputPath == "-" {
		if _, err := stdout.Write(generated); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if err := os.WriteFile(outputPath, generated, 0o644); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", outputPath, err)
		return 1
	}
	return 0
}

func usage(output io.Writer) {
	fmt.Fprintln(output, "usage: cmdwire <check|format> [file ...]")
	fmt.Fprintln(output, "       cmdwire schema check <schema ...>")
	fmt.Fprintln(output, "       cmdwire schema generate-go <package> <output|-> <schema ...>")
	fmt.Fprintln(output, "       cmdwire schema generate-rust <output|-> <schema ...>")
}

func process(action, name string, input io.Reader, output io.Writer) error {
	decoder := cmdwire.NewDecoder(input)
	for line := 1; ; line++ {
		record, err := decoder.Decode()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s:%d: %w", name, line, err)
		}
		if action == "format" {
			formatted, err := cmdwire.Format(record)
			if err != nil {
				return fmt.Errorf("%s:%d: %w", name, line, err)
			}
			if _, err := fmt.Fprintln(output, formatted); err != nil {
				return fmt.Errorf("%s:%d: writing output: %w", name, line, err)
			}
		}
	}
}
