package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRunConvertsStdin(t *testing.T) {
	input := "name: Legacy\nsteps:\n  - run: printf ok\n"
	var stdout, stderr bytes.Buffer
	rc := run([]string{"--from", "main-v0.1", "-"}, strings.NewReader(input), &stdout, &stderr)
	if rc != 0 {
		t.Fatalf("run() = %d, stderr=%q", rc, stderr.String())
	}
	for _, want := range []string{"pxp_version: \"1.0\"", "name: Legacy", "run:", "- printf ok"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunUsageErrorsExitTwo(t *testing.T) {
	tests := [][]string{
		nil,
		{"input.yaml"},
		{"--from", "auto", "input.yaml"},
		{"--from", "main-v0.1"},
		{"--from", "main-v0.1", "one", "two"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		if rc := run(args, strings.NewReader(""), &stdout, &stderr); rc != 2 {
			t.Errorf("run(%q) = %d, want 2", args, rc)
		}
	}
}

func TestRunHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"version"}, {"--version"}} {
		var stdout, stderr bytes.Buffer
		if rc := run(args, strings.NewReader(""), &stdout, &stderr); rc != 0 {
			t.Errorf("run(%q) = %d, want 0; stderr=%q", args, rc, stderr.String())
		}
	}
}

func TestRunConversionErrorExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	rc := run([]string{"--from", "main-v0.1", "-"}, strings.NewReader("not: valid: yaml"), &stdout, &stderr)
	if rc != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Error:") {
		t.Fatalf("rc=%d stdout=%q stderr=%q", rc, stdout.String(), stderr.String())
	}
}

func TestRunWriteErrorExitsSeventy(t *testing.T) {
	input := "name: Legacy\nsteps:\n  - run: printf ok\n"
	var stderr bytes.Buffer
	rc := run([]string{"--from", "main-v0.1", "-"}, strings.NewReader(input), errorWriter{}, &stderr)
	if rc != 70 || !strings.Contains(stderr.String(), "Internal error") {
		t.Fatalf("rc=%d stderr=%q", rc, stderr.String())
	}
}

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("forced write failure") }
