//go:build darwin || linux

package integration_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConverterDoesNotExecuteAndProducesRunnableMainV01(t *testing.T) {
	work := t.TempDir()
	marker := filepath.Join(work, "must-not-exist-during-conversion")
	legacy := "name: Converted main\nsteps:\n  - run: touch " + marker + "\n"
	result := invokeConverter(t, work, []string{"--from", "main-v0.1", "-"}, legacy)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, marker)
	for _, want := range []string{"pxp_version: \"1.0\"", "name: Converted main", "run:"} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("canonical output missing %q:\n%s", want, result.stdout)
		}
	}
	for _, forbidden := range []string{"\nplan:", "\ntype:", "\ncommands:", "\ncommand:", "\ndiff:", "\nid:"} {
		if strings.Contains(result.stdout, forbidden) {
			t.Errorf("canonical output contains legacy field %q:\n%s", forbidden, result.stdout)
		}
	}

	repo := newRepository(t)
	executed := invoke(t, repo, []string{"run", "-"}, result.stdout, nil)
	if executed.code != 0 {
		t.Fatalf("converted plan exit=%d stdout=%q stderr=%q", executed.code, executed.stdout, executed.stderr)
	}
	assertExists(t, marker)
}

func TestConverterDevelopProfileNormalizesHistoricalAndDocumentedShellForms(t *testing.T) {
	legacy := `pxp_version: "0.1"
plan:
  task: Develop plan
  steps:
    - id: 1
      type: shell
      commands:
        - printf one
        - printf two
    - id: singular
      type: shell
      command: printf three
`
	result := invokeConverter(t, t.TempDir(), []string{"--from", "develop-v0.1", "-"}, legacy)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	for _, command := range []string{"printf one", "printf two", "printf three"} {
		if !strings.Contains(result.stdout, command) {
			t.Errorf("output missing %q:\n%s", command, result.stdout)
		}
	}
	for _, legacyField := range []string{"plan:", "id:", "type:", "commands:", "command:"} {
		if strings.Contains(result.stdout, legacyField) {
			t.Errorf("output contains legacy field %q:\n%s", legacyField, result.stdout)
		}
	}
}

func TestConverterRequiresExplicitProfileAndNeverGuesses(t *testing.T) {
	input := "name: Legacy\nsteps: [{run: printf ok}]\n"
	for _, args := range [][]string{{"-"}, {"--from", "auto", "-"}, {"--from", "develop-v0.1", "-"}} {
		result := invokeConverter(t, t.TempDir(), args, input)
		if result.code != 2 || result.stdout != "" {
			t.Errorf("args=%q exit=%d stdout=%q stderr=%q", args, result.code, result.stdout, result.stderr)
		}
	}
}

func TestConverterNameOverrideAndStrictFailures(t *testing.T) {
	unnamed := "steps:\n  - run: printf ok\n"
	result := invokeConverter(t, t.TempDir(), []string{"--from", "main-v0.1", "-"}, unnamed)
	if result.code != 2 || result.stdout != "" {
		t.Fatalf("unnamed exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	result = invokeConverter(t, t.TempDir(), []string{"--from", "main-v0.1", "--name", "Override", "-"}, unnamed)
	if result.code != 0 || !strings.Contains(result.stdout, "name: Override") {
		t.Fatalf("override exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	invalid := []string{
		"name: x\nunknown: y\nsteps: [{run: echo}]\n",
		"name: x\nsteps: &s [{run: echo}]\n",
		"name: x\nsteps: [{run: echo, patch: diff}]\n",
	}
	for _, input := range invalid {
		result := invokeConverter(t, t.TempDir(), []string{"--from", "main-v0.1", "-"}, input)
		if result.code != 2 || result.stdout != "" {
			t.Errorf("invalid input exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
	}
}

func TestConverterWorksOutsideGitAndWritesOnlyStdout(t *testing.T) {
	work := t.TempDir()
	result := invokeConverter(t, work, []string{"--from", "main-v0.1", "-"}, "name: x\nsteps: [{run: printf ok}]\n")
	if result.code != 0 || result.stderr != "" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("converter wrote files: %v", entries)
	}
}

func invokeConverter(t *testing.T, cwd string, args []string, stdin string) commandResult {
	t.Helper()
	cmd := exec.Command(converterBinary, args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return commandResult{code: exitCode(err), stdout: stdout.String(), stderr: stderr.String()}
}
