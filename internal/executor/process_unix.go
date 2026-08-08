//go:build darwin || linux

package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/otar/pxp/internal/interrupt"
)

const processShutdownGrace = 2 * time.Second

type BashRunner struct {
	path string
}

func NewBashRunner() (*BashRunner, error) {
	path, err := exec.LookPath("bash")
	if err != nil {
		return nil, fmt.Errorf("bash is required: %w", err)
	}
	if err := checkBashVersion(path); err != nil {
		return nil, err
	}
	return &BashRunner{path: path}, nil
}

type CommandError struct {
	ExitCode int
	Err      error
}

func (e *CommandError) Error() string {
	if e.ExitCode >= 0 {
		return fmt.Sprintf("command exited with code %d", e.ExitCode)
	}
	return fmt.Sprintf("command failed: %v", e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }

func (r *BashRunner) Run(ctx context.Context, root, command string, stdout, stderr io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cmd := exec.Command(
		r.path,
		"--noprofile",
		"--norc",
		"-e",
		"-u",
		"-o", "pipefail",
		"-c", command,
	)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = cleanEnvironment(os.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return &CommandError{ExitCode: -1, Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return commandResult(err)
	case <-ctx.Done():
		signal := syscall.SIGTERM
		if received, ok := interrupt.Signal(ctx); ok {
			signal = received
		}
		_ = syscall.Kill(-cmd.Process.Pid, signal)
		select {
		case <-done:
			// Non-interactive shells may start background processes that ignore
			// SIGINT. The shell can exit while those processes remain in its
			// process group, so finish the canceled group explicitly.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-time.After(processShutdownGrace):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		return ctx.Err()
	}
}

func checkBashVersion(path string) error {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return fmt.Errorf("inspect Bash version: %w", err)
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	var major, minor int
	if _, err := fmt.Sscanf(line, "GNU bash, version %d.%d", &major, &minor); err != nil {
		return fmt.Errorf("parse Bash version from %q", line)
	}
	if major < 3 || major == 3 && minor < 2 {
		return fmt.Errorf("bash 3.2 or newer is required (found %d.%d)", major, minor)
	}
	return nil
}

func cleanEnvironment(environment []string) []string {
	cleaned := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "BASH_ENV", "ENV", "CDPATH":
			continue
		default:
			cleaned = append(cleaned, entry)
		}
	}
	return cleaned
}

func commandResult(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &CommandError{ExitCode: exitErr.ExitCode(), Err: err}
	}
	return &CommandError{ExitCode: -1, Err: err}
}
