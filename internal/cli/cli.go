package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/otar/pxp/internal/executor"
	"github.com/otar/pxp/internal/interrupt"
	"github.com/otar/pxp/internal/output"
	"github.com/otar/pxp/internal/plan"
	"github.com/otar/pxp/internal/repository"
	"github.com/otar/pxp/internal/version"
)

const rootHelp = `pxp - execute reviewed PXP plans

Usage:
  pxp run [--allow-dirty] [--expect-sha256 HASH] <plan.pxp.yaml|->
  pxp help [run|version]
  pxp version

Commands:
  run      Validate and execute a plan
  help     Show help
  version  Show version information
`

const runHelp = `Usage:
  pxp run [--allow-dirty] [--expect-sha256 HASH] <plan.pxp.yaml|->

Arguments:
  plan.pxp.yaml  Plan file, resolved from the invocation directory
  -              Read the plan from standard input

Options:
  --allow-dirty          Allow pre-existing tracked changes; locking and safety checks remain enabled
  --expect-sha256 HASH   Require the exact lowercase SHA-256 of the plan input
  -h, --help             Show this help
`

const versionHelp = `Usage:
  pxp version
`

// Run executes the CLI and returns a stable process exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string) int {
	printer := output.New(stderr)
	if len(args) == 0 {
		fmt.Fprint(stderr, rootHelp)
		return ExitUsage
	}

	switch args[0] {
	case "-h", "--help":
		if len(args) != 1 {
			printer.Error("usage", "--help does not accept arguments")
			return ExitUsage
		}
		fmt.Fprint(stdout, rootHelp)
		return ExitOK
	case "--version":
		if len(args) != 1 {
			printer.Error("usage", "--version does not accept arguments")
			return ExitUsage
		}
		fmt.Fprintln(stdout, version.String())
		return ExitOK
	case "help":
		return help(args[1:], stdout, printer)
	case "version":
		if len(args) != 1 {
			printer.Error("usage", "version does not accept arguments")
			return ExitUsage
		}
		fmt.Fprintln(stdout, version.String())
		return ExitOK
	case "run":
		return runPlan(ctx, args[1:], stdin, stdout, stderr, cwd, printer)
	default:
		printer.Error("usage", fmt.Sprintf("unknown command %q", args[0]))
		return ExitUsage
	}
}

func help(args []string, stdout io.Writer, printer *output.Printer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, rootHelp)
		return ExitOK
	}
	if len(args) != 1 {
		printer.Error("usage", "help accepts at most one command")
		return ExitUsage
	}
	switch args[0] {
	case "run":
		fmt.Fprint(stdout, runHelp)
	case "version":
		fmt.Fprint(stdout, versionHelp)
	case "help":
		fmt.Fprint(stdout, rootHelp)
	default:
		printer.Error("usage", fmt.Sprintf("unknown help topic %q", args[0]))
		return ExitUsage
	}
	return ExitOK
}

func runPlan(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, cwd string, printer *output.Printer) int {
	options, err := parseRunArgs(args)
	if err != nil {
		printer.Error("usage", err.Error())
		return ExitUsage
	}
	if options.showHelp {
		fmt.Fprint(stdout, runHelp)
		return ExitOK
	}
	if code := interrupted(ctx); code != ExitOK {
		printer.Error("interrupted", "execution canceled")
		return code
	}

	document, err := readPlanContext(ctx, options.input, cwd, stdin)
	if err != nil {
		if code := interrupted(ctx); code != ExitOK {
			printer.Error("interrupted", "execution canceled")
			return code
		}
		printer.Error("plan", err.Error())
		return ExitPlan
	}
	if options.expectedSHA256 != "" && document.SHA256 != options.expectedSHA256 {
		printer.Error("precondition", fmt.Sprintf("plan SHA-256 mismatch: expected %s, got %s", options.expectedSHA256, document.SHA256))
		return ExitPrecondition
	}

	repo, err := repository.Open(ctx, cwd)
	if err != nil {
		if code := interrupted(ctx); code != ExitOK {
			printer.Error("interrupted", "execution canceled")
			return code
		}
		printer.Error("precondition", err.Error())
		return ExitPrecondition
	}
	runner, err := executor.NewBashRunner()
	if err != nil {
		printer.Error("precondition", err.Error())
		return ExitPrecondition
	}

	lock, err := repository.AcquireLock(repo.LockPath)
	if err != nil {
		printer.Error("precondition", err.Error())
		return ExitPrecondition
	}

	if document.Repository != nil {
		if err := repo.CheckHead(ctx, document.Repository.Head); err != nil {
			releaseErr := lock.Release()
			if code := interrupted(ctx); code != ExitOK {
				printer.Error("interrupted", "execution canceled")
				return code
			}
			printer.Error("precondition", err.Error())
			if releaseErr != nil {
				printer.Error("internal", releaseErr.Error())
			}
			return ExitPrecondition
		}
	}
	if !options.allowDirty {
		if err := repo.CheckClean(ctx); err != nil {
			releaseErr := lock.Release()
			if code := interrupted(ctx); code != ExitOK {
				printer.Error("interrupted", "execution canceled")
				return code
			}
			printer.Error("precondition", err.Error())
			if releaseErr != nil {
				printer.Error("internal", releaseErr.Error())
			}
			return ExitPrecondition
		}
	}
	for stepIndex := range document.Steps {
		step := &document.Steps[stepIndex]
		if step.Patch == nil {
			continue
		}
		if err := repo.CheckPatchPolicy(ctx, *step.Patch); err != nil {
			releaseErr := lock.Release()
			if code := interrupted(ctx); code != ExitOK {
				printer.Error("interrupted", "execution canceled")
				return code
			}
			printer.Error("plan", fmt.Sprintf("steps[%d]: %v", stepIndex+1, err))
			if releaseErr != nil {
				printer.Error("internal", releaseErr.Error())
			}
			return ExitPlan
		}
	}

	engine := executor.New(repo, runner, printer, repo.Root, stdout, stderr)
	executionErr := engine.Execute(ctx, document)
	releaseErr := lock.Release()

	if code := interrupted(ctx); code != ExitOK {
		printer.Error("interrupted", "execution canceled")
		if releaseErr != nil {
			printer.Error("internal", releaseErr.Error())
		}
		return code
	}
	if executionErr != nil {
		printer.Error("execution", executionErr.Error())
		if releaseErr != nil {
			printer.Error("internal", releaseErr.Error())
		}
		return ExitExecution
	}
	if releaseErr != nil {
		printer.Error("internal", releaseErr.Error())
		return ExitInternal
	}
	return ExitOK
}

type runOptions struct {
	allowDirty     bool
	expectedSHA256 string
	input          string
	showHelp       bool
}

func parseRunArgs(args []string) (runOptions, error) {
	var options runOptions
	endFlags := false
	seenAllowDirty := false
	seenExpectedSHA256 := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !endFlags {
			switch arg {
			case "--":
				endFlags = true
				continue
			case "-h", "--help":
				options.showHelp = true
				continue
			case "--allow-dirty":
				if seenAllowDirty {
					return runOptions{}, fmt.Errorf("--allow-dirty may be specified only once")
				}
				seenAllowDirty = true
				options.allowDirty = true
				continue
			case "--expect-sha256":
				if seenExpectedSHA256 {
					return runOptions{}, fmt.Errorf("--expect-sha256 may be specified only once")
				}
				if index+1 >= len(args) {
					return runOptions{}, fmt.Errorf("--expect-sha256 requires a hash")
				}
				seenExpectedSHA256 = true
				index++
				options.expectedSHA256 = args[index]
				if !validSHA256(options.expectedSHA256) {
					return runOptions{}, fmt.Errorf("--expect-sha256 must be 64 lowercase hexadecimal characters")
				}
				continue
			}
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return runOptions{}, fmt.Errorf("unknown run option %q", arg)
			}
		}
		if options.input != "" {
			return runOptions{}, fmt.Errorf("run accepts exactly one plan input")
		}
		options.input = arg
	}
	if options.showHelp {
		if options.input != "" || options.allowDirty || options.expectedSHA256 != "" {
			return runOptions{}, fmt.Errorf("--help cannot be combined with a plan or run options")
		}
		return runOptions{showHelp: true}, nil
	}
	if options.input == "" {
		return runOptions{}, fmt.Errorf("missing plan input")
	}
	return options, nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}

type planReadResult struct {
	document *plan.Plan
	err      error
}

func readPlanContext(ctx context.Context, input, cwd string, stdin io.Reader) (*plan.Plan, error) {
	result := make(chan planReadResult, 1)
	go func() {
		document, err := readPlan(input, cwd, stdin)
		result <- planReadResult{document: document, err: err}
	}()

	select {
	case completed := <-result:
		return completed.document, completed.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func readPlan(input, cwd string, stdin io.Reader) (*plan.Plan, error) {
	if input == "-" {
		document, err := plan.Read(stdin)
		if err != nil {
			return nil, err
		}
		return document, nil
	}

	path := input
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect plan %q: %w", input, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("plan %q must be a non-symlink regular file", input)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plan %q: %w", input, err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("inspect open plan %q: %w", input, err)
	}
	if !fileInfo.Mode().IsRegular() || !os.SameFile(pathInfo, fileInfo) {
		file.Close()
		return nil, fmt.Errorf("plan %q must be a non-symlink regular file", input)
	}
	defer file.Close()
	document, err := plan.Read(file)
	if err != nil {
		return nil, err
	}
	return document, nil
}

func interrupted(ctx context.Context) int {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return interrupt.ExitCode(ctx)
	}
	return ExitOK
}
