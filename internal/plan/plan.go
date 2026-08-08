package plan

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const (
	ProtocolVersion = "1.0"
	MaxNameBytes    = 1024
	MaxSteps        = 1000
	MaxRunCommands  = 100
	MaxCommandBytes = 64 << 10
)

// Plan is the validated PXP v1 execution document.
type Plan struct {
	PXPVersion string `yaml:"pxp_version"`
	Name       string `yaml:"name"`
	Steps      []Step `yaml:"steps"`
	SHA256     string `yaml:"-"`
}

// Step contains exactly one executable operation. A nil Run is distinct from
// an explicitly provided, but empty, run list.
type Step struct {
	Name  *string  `yaml:"name,omitempty"`
	Run   []string `yaml:"run,omitempty"`
	Patch *string  `yaml:"patch,omitempty"`
}

func (p *Plan) validate() error {
	if p.PXPVersion != ProtocolVersion {
		if p.PXPVersion == "" {
			return fmt.Errorf("pxp_version is required and must equal %q", ProtocolVersion)
		}
		return fmt.Errorf("unsupported pxp_version %q (expected %q)", p.PXPVersion, ProtocolVersion)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required and must not be empty")
	}
	if err := validateHumanName("name", p.Name); err != nil {
		return err
	}
	if len(p.Steps) == 0 {
		return fmt.Errorf("steps is required and must contain at least one step")
	}
	if len(p.Steps) > MaxSteps {
		return fmt.Errorf("steps contains %d entries; maximum is %d", len(p.Steps), MaxSteps)
	}

	for i := range p.Steps {
		if err := p.Steps[i].validate(); err != nil {
			return fmt.Errorf("steps[%d]: %w", i+1, err)
		}
	}
	return nil
}

func (s *Step) validate() error {
	if s.Name != nil {
		if err := validateHumanName("name", *s.Name); err != nil {
			return err
		}
	}

	hasRun := s.Run != nil
	hasPatch := s.Patch != nil
	if hasRun == hasPatch {
		return fmt.Errorf("exactly one of run or patch is required")
	}

	if hasRun {
		if len(s.Run) == 0 {
			return fmt.Errorf("run must contain at least one command")
		}
		if len(s.Run) > MaxRunCommands {
			return fmt.Errorf("run contains %d commands; maximum is %d", len(s.Run), MaxRunCommands)
		}
		for i, command := range s.Run {
			if strings.TrimSpace(command) == "" {
				return fmt.Errorf("run[%d] must not be empty", i+1)
			}
			if strings.ContainsRune(command, '\x00') {
				return fmt.Errorf("run[%d] contains NUL", i+1)
			}
			if len(command) > MaxCommandBytes {
				return fmt.Errorf("run[%d] is %d bytes; maximum is %d", i+1, len(command), MaxCommandBytes)
			}
		}
		return nil
	}

	if strings.TrimSpace(*s.Patch) == "" {
		return fmt.Errorf("patch must not be empty")
	}
	if strings.ContainsRune(*s.Patch, '\x00') {
		return fmt.Errorf("patch contains NUL")
	}
	if err := validatePatch(*s.Patch); err != nil {
		return fmt.Errorf("invalid patch: %w", err)
	}
	return nil
}

func validateHumanName(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be blank", field)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains NUL", field)
	}
	if len(value) > MaxNameBytes {
		return fmt.Errorf("%s is %d bytes; maximum is %d", field, len(value), MaxNameBytes)
	}
	return nil
}

func validatePatch(diff string) error {
	if err := validateRawGitHeaders(diff); err != nil {
		return err
	}
	files, preamble, err := gitdiff.Parse(strings.NewReader(diff))
	if err != nil {
		return err
	}
	if strings.TrimSpace(preamble) != "" {
		return fmt.Errorf("content before the first file header is not allowed")
	}
	if len(files) == 0 {
		return fmt.Errorf("no file changes found")
	}

	for _, file := range files {
		if file.IsBinary || file.BinaryFragment != nil || file.ReverseBinaryFragment != nil {
			return fmt.Errorf("binary patches are not supported")
		}
		if file.IsRename || file.IsCopy {
			return fmt.Errorf("rename and copy patches are not supported")
		}
		if !file.IsNew && !file.IsDelete && len(file.TextFragments) == 0 {
			return fmt.Errorf("pure mode changes are not supported")
		}
		if file.OldMode != 0 && file.NewMode != 0 && file.OldMode != file.NewMode {
			return fmt.Errorf("content and mode changes cannot be combined")
		}
		for _, item := range []struct {
			label string
			mode  os.FileMode
		}{
			{label: "old", mode: file.OldMode},
			{label: "new", mode: file.NewMode},
		} {
			if !allowedRegularMode(item.mode) {
				return fmt.Errorf("%s mode %06o is not a supported regular-file mode", item.label, item.mode)
			}
		}
		for _, fragment := range file.TextFragments {
			if err := fragment.Validate(); err != nil {
				return fmt.Errorf("invalid hunk: %w", err)
			}
		}
		if file.OldName == "" && file.NewName == "" {
			return fmt.Errorf("file change has no path")
		}
		for _, name := range []string{file.OldName, file.NewName} {
			if name == "" {
				continue
			}
			if err := validatePatchPath(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRawGitHeaders(diff string) error {
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "diff --git ") {
			continue
		}
		header := strings.TrimPrefix(line, "diff --git ")
		if strings.Contains(header, "//") || strings.Contains(header, "/./") {
			return fmt.Errorf("git file header contains an empty or dot path component")
		}
	}
	return nil
}

// DisplayName returns the optional human-readable step name.
func (s *Step) DisplayName() string {
	if s.Name == nil {
		return ""
	}
	return *s.Name
}

func allowedRegularMode(mode os.FileMode) bool {
	switch mode {
	case 0, os.FileMode(0o100644), os.FileMode(0o100755):
		return true
	default:
		return false
	}
}

func validatePatchPath(name string) error {
	if strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("path contains NUL")
	}
	if strings.Contains(name, `\`) {
		return fmt.Errorf("path %q contains a backslash", name)
	}
	if path.IsAbs(name) || strings.HasPrefix(name, "//") {
		return fmt.Errorf("path %q is absolute", name)
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || part == "." {
			return fmt.Errorf("path %q contains an empty or dot component", name)
		}
		if part == ".." {
			return fmt.Errorf("path %q escapes the repository", name)
		}
		if strings.EqualFold(part, ".git") {
			return fmt.Errorf("path %q targets Git metadata", name)
		}
	}
	cleaned := path.Clean(name)
	if cleaned == "." || cleaned == "" {
		return fmt.Errorf("path %q is not a file path", name)
	}
	return nil
}
