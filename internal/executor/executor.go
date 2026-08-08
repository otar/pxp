package executor

import (
	"context"
	"fmt"
	"io"

	"github.com/otar/pxp/internal/output"
	"github.com/otar/pxp/internal/plan"
)

type PatchRepository interface {
	CheckPatch(context.Context, string) error
	ApplyPatch(context.Context, string) error
}

type CommandRunner interface {
	Run(context.Context, string, string, io.Writer, io.Writer) error
}

// Failure identifies the exact operation that stopped execution.
type Failure struct {
	StepIndex    int
	StepName     string
	CommandIndex int
	Operation    string
	Err          error
}

func (e *Failure) Error() string {
	step := fmt.Sprintf("step %d", e.StepIndex)
	if e.StepName != "" {
		step += fmt.Sprintf(" (%q)", e.StepName)
	}
	if e.CommandIndex != 0 {
		return fmt.Sprintf("%s, command %d: %v", step, e.CommandIndex, e.Err)
	}
	return fmt.Sprintf("%s, %s: %v", step, e.Operation, e.Err)
}

func (e *Failure) Unwrap() error { return e.Err }

type Executor struct {
	repository PatchRepository
	runner     CommandRunner
	printer    *output.Printer
	root       string
	stdout     io.Writer
	stderr     io.Writer
}

func New(repository PatchRepository, runner CommandRunner, printer *output.Printer, root string, stdout, stderr io.Writer) *Executor {
	return &Executor{
		repository: repository,
		runner:     runner,
		printer:    printer,
		root:       root,
		stdout:     stdout,
		stderr:     stderr,
	}
}

func (e *Executor) Execute(ctx context.Context, document *plan.Plan) error {
	e.printer.Plan(document.Name, len(document.Steps), document.SHA256)
	for stepIndex := range document.Steps {
		if err := ctx.Err(); err != nil {
			return err
		}

		step := &document.Steps[stepIndex]
		kind := "patch"
		if step.Run != nil {
			kind = "run"
		}
		stepName := step.DisplayName()
		e.printer.Step(stepIndex+1, len(document.Steps), kind, stepName)

		if step.Run != nil {
			for commandIndex, command := range step.Run {
				e.printer.Command(commandIndex+1, len(step.Run))
				if err := e.runner.Run(ctx, e.root, command, e.stdout, e.stderr); err != nil {
					return &Failure{
						StepIndex:    stepIndex + 1,
						StepName:     stepName,
						CommandIndex: commandIndex + 1,
						Operation:    "run",
						Err:          err,
					}
				}
			}
			e.printer.StepDone()
			continue
		}

		if err := e.repository.CheckPatch(ctx, *step.Patch); err != nil {
			return &Failure{StepIndex: stepIndex + 1, StepName: stepName, Operation: "patch preflight", Err: err}
		}
		if err := e.repository.ApplyPatch(ctx, *step.Patch); err != nil {
			return &Failure{StepIndex: stepIndex + 1, StepName: stepName, Operation: "patch apply", Err: err}
		}
		e.printer.StepDone()
	}
	e.printer.Success()
	return nil
}
