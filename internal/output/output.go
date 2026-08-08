package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

// Printer emits human-readable progress and diagnostics.
type Printer struct {
	lifecycle io.Writer
	color     bool
}

func New(lifecycle io.Writer) *Printer {
	return &Printer{
		lifecycle: lifecycle,
		color:     os.Getenv("NO_COLOR") == "" && isTerminal(lifecycle),
	}
}

func isTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (p *Printer) Plan(name string, steps int, digest string) {
	fmt.Fprintf(p.lifecycle, "%s%sPlan:%s %s (%d steps)\n", p.bold(), p.cyan(), p.reset(), safeLabel(name), steps)
	fmt.Fprintf(p.lifecycle, "Plan SHA-256: %s\n", digest)
}

func (p *Printer) Step(index, total int, kind, name string) {
	label := name
	if strings.TrimSpace(label) == "" {
		label = kind
	}
	fmt.Fprintf(p.lifecycle, "[%d/%d] %s: %s\n", index, total, kind, safeLabel(label))
}

func (p *Printer) Command(index, total int) {
	fmt.Fprintf(p.lifecycle, "  command %d/%d\n", index, total)
}

func (p *Printer) StepDone() {
	fmt.Fprintf(p.lifecycle, "  %sOK%s\n", p.green(), p.reset())
}

func (p *Printer) Success() {
	fmt.Fprintf(p.lifecycle, "%s%sPlan completed successfully.%s\n", p.bold(), p.green(), p.reset())
}

func (p *Printer) Error(category, message string) {
	fmt.Fprintf(p.lifecycle, "%s%serror[%s]:%s %s\n", p.bold(), p.red(), category, p.reset(), sanitizeDiagnostic(message))
}

func safeLabel(value string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '\ufffd'
		}
		return r
	}, value)
}

func sanitizeDiagnostic(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\t':
			return r
		}
		if r < 0x20 || r == 0x7f {
			return '\ufffd'
		}
		return r
	}, value)
}

func (p *Printer) bold() string {
	if p.color {
		return ansiBold
	}
	return ""
}

func (p *Printer) cyan() string {
	if p.color {
		return ansiCyan
	}
	return ""
}

func (p *Printer) green() string {
	if p.color {
		return ansiGreen
	}
	return ""
}

func (p *Printer) red() string {
	if p.color {
		return ansiRed
	}
	return ""
}

func (p *Printer) reset() string {
	if p.color {
		return ansiReset
	}
	return ""
}
