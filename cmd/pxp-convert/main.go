package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/otar/pxp/internal/legacyconvert"
	"github.com/otar/pxp/internal/version"
)

const maxInputBytes = 16 << 20

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		fmt.Fprintln(stdout, version.For("pxp-convert"))
		return 0
	}

	fs := flag.NewFlagSet("pxp-convert", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profile := fs.String("from", "", "legacy format: main-v0.1 or develop-v0.1")
	name := fs.String("name", "", "override/fallback task name")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: pxp-convert --from main-v0.1|develop-v0.1 [--name TASK] <file|->")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *profile != legacyconvert.ProfileMainV01 && *profile != legacyconvert.ProfileDevelopV01 {
		fmt.Fprintln(stderr, "Error: --from must be main-v0.1 or develop-v0.1")
		fs.Usage()
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Error: exactly one input file or - is required")
		fs.Usage()
		return 2
	}

	input, err := readInput(fs.Arg(0), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 2
	}
	converted, err := legacyconvert.Convert(input, *profile, *name)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 2
	}
	if _, err := stdout.Write(converted); err != nil {
		fmt.Fprintf(stderr, "Internal error: write output: %v\n", err)
		return 70
	}
	return 0
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return readLimited(stdin)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input %q is not a regular file", path)
	}
	return readLimited(f)
}

func readLimited(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}
	return data, nil
}
