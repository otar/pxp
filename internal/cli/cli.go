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
  pxp run [--allow-dirty] <plan.pxp.yaml|->
  pxp help [run|version]
  pxp version

Commands:
  run      Validate and execute a plan
  help     Show help
  version  Show version information
`

const runHelp = `Usage:
  pxp run [--allow-dirty] <plan.pxp.yaml|->

Arguments:
  plan.pxp.yaml  Plan file, resolved from the invocation directory
  -              Read the plan from standard input

Options:
  --allow-dirty  Allow pre-existing tracked changes; locking and safety checks remain enabled
  -h, --help     Show this help
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
	allowDirty, input, showHelp, err := parseRunArgs(args)
	if err != nil {
		printer.Error("usage", err.Error())
		return ExitUsage
	}
	if showHelp {
		fmt.Fprint(stdout, runHelp)
		return ExitOK
	}
	if code := interrupted(ctx); code != ExitOK {
		printer.Error("interrupted", "execution canceled")
		return code
	}

	document, err := readPlanContext(ctx, input, cwd, stdin)
	if err != nil {
		if code := interrupted(ctx); code != ExitOK {
			printer.Error("interrupted", "execution canceled")
			return code
		}
		printer.Error("plan", err.Error())
		return ExitPlan
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

	if !allowDirty {
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

func parseRunArgs(args []string) (allowDirty bool, input string, showHelp bool, err error) {
	endFlags := false
	seenAllowDirty := false
	for _, arg := range args {
		if !endFlags {
			switch arg {
			case "--":
				endFlags = true
				continue
			case "-h", "--help":
				showHelp = true
				continue
			case "--allow-dirty":
				if seenAllowDirty {
					return false, "", false, fmt.Errorf("--allow-dirty may be specified only once")
				}
				seenAllowDirty = true
				allowDirty = true
				continue
			}
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return false, "", false, fmt.Errorf("unknown run option %q", arg)
			}
		}
		if input != "" {
			return false, "", false, fmt.Errorf("run accepts exactly one plan input")
		}
		input = arg
	}
	if showHelp {
		if input != "" || allowDirty {
			return false, "", false, fmt.Errorf("--help cannot be combined with a plan or --allow-dirty")
		}
		return false, "", true, nil
	}
	if input == "" {
		return false, "", false, fmt.Errorf("missing plan input")
	}
	return allowDirty, input, false, nil
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
