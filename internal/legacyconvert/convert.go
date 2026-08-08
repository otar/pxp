package legacyconvert

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"
)

const (
	ProfileMainV01    = "main-v0.1"
	ProfileDevelopV01 = "develop-v0.1"
	maxInputBytes     = 16 << 20
	maxSteps          = 1000
	maxCommands       = 100
	maxCommandBytes   = 64 << 10
)

// Convert converts one explicitly selected historical format into canonical
// PXP v1 YAML. It never executes input and never guesses the source format.
func Convert(input []byte, profile, nameOverride string) ([]byte, error) {
	if len(input) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d bytes", maxInputBytes)
	}
	if !utf8.Valid(input) {
		return nil, errors.New("input is not valid UTF-8")
	}
	if err := rejectComplexYAML(input); err != nil {
		return nil, err
	}
	if err := validateLegacyScalarTypes(input, profile); err != nil {
		return nil, err
	}

	var (
		out outputDocument
		err error
	)
	switch profile {
	case ProfileMainV01:
		var legacy mainDocument
		if err := decodeOne(input, &legacy); err != nil {
			return nil, err
		}
		out, err = convertMain(legacy, nameOverride)
	case ProfileDevelopV01:
		var legacy developDocument
		if err := decodeOne(input, &legacy); err != nil {
			return nil, err
		}
		out, err = convertDevelop(legacy, nameOverride)
	default:
		return nil, fmt.Errorf("unsupported source profile %q", profile)
	}
	if err != nil {
		return nil, err
	}
	if err := validateOutput(out); err != nil {
		return nil, fmt.Errorf("converted plan is invalid: %w", err)
	}
	encoded, err := yaml.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode converted plan: %w", err)
	}
	return encoded, nil
}

type outputDocument struct {
	Version string       `yaml:"pxp_version"`
	Name    string       `yaml:"name"`
	Steps   []outputStep `yaml:"steps"`
}

type outputStep struct {
	Name  string   `yaml:"name,omitempty"`
	Run   []string `yaml:"run,omitempty"`
	Patch string   `yaml:"patch,omitempty"`
}

type mainDocument struct {
	Name  string     `yaml:"name"`
	Steps []mainStep `yaml:"steps"`
}

type mainStep struct {
	Name  string  `yaml:"name"`
	Run   *string `yaml:"run"`
	Patch *string `yaml:"patch"`
}

type developDocument struct {
	Version string       `yaml:"pxp_version"`
	Plan    *developPlan `yaml:"plan"`
}

type developPlan struct {
	Task  string        `yaml:"task"`
	Steps []developStep `yaml:"steps"`
}

type developStep struct {
	ID       any       `yaml:"id"`
	Name     string    `yaml:"name"`
	Type     string    `yaml:"type"`
	Commands *[]string `yaml:"commands"`
	Command  *string   `yaml:"command"`
	Diff     *string   `yaml:"diff"`
}

func decodeOne(input []byte, dst any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(input), yaml.Strict())
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode legacy YAML: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple YAML documents are not allowed")
		}
		return fmt.Errorf("decode trailing YAML: %w", err)
	}
	return nil
}

func rejectComplexYAML(input []byte) error {
	tokens := lexer.Tokenize(string(input))
	if invalid := tokens.InvalidToken(); invalid != nil {
		return fmt.Errorf("decode legacy YAML: %s", invalid.Error)
	}
	for _, tok := range tokens {
		switch tok.Type {
		case token.AnchorType, token.AliasType:
			return errors.New("anchors and aliases are not allowed")
		case token.MergeKeyType:
			return errors.New("merge keys are not allowed")
		case token.TagType, token.DirectiveType:
			return errors.New("explicit YAML tags and directives are not allowed")
		}
	}
	return nil
}

func validateLegacyScalarTypes(input []byte, profile string) error {
	var raw map[string]any
	if err := yaml.UnmarshalWithOptions(input, &raw, yaml.Strict()); err != nil {
		return fmt.Errorf("decode legacy YAML: %w", err)
	}
	switch profile {
	case ProfileMainV01:
		if err := optionalStringField(raw, "name", "name"); err != nil {
			return err
		}
		steps, err := objectSequence(raw["steps"], "steps")
		if err != nil {
			return err
		}
		for i, step := range steps {
			path := fmt.Sprintf("steps[%d]", i)
			for _, key := range []string{"name", "run", "patch"} {
				if err := optionalStringField(step, key, path+"."+key); err != nil {
					return err
				}
			}
		}
	case ProfileDevelopV01:
		if err := requiredStringField(raw, "pxp_version", "pxp_version"); err != nil {
			return err
		}
		planMap, ok := raw["plan"].(map[string]any)
		if !ok {
			return errors.New("plan must be a mapping")
		}
		if err := requiredStringField(planMap, "task", "plan.task"); err != nil {
			return err
		}
		steps, err := objectSequence(planMap["steps"], "plan.steps")
		if err != nil {
			return err
		}
		for i, step := range steps {
			path := fmt.Sprintf("plan.steps[%d]", i)
			for _, key := range []string{"name", "type", "command", "diff"} {
				if err := optionalStringField(step, key, path+"."+key); err != nil {
					return err
				}
			}
			if id, exists := step["id"]; exists && !isStringOrInteger(id) {
				return fmt.Errorf("%s.id must be a string or integer", path)
			}
			if commands, exists := step["commands"]; exists {
				values, ok := commands.([]any)
				if !ok {
					return fmt.Errorf("%s.commands must be a sequence", path)
				}
				for j, value := range values {
					if _, ok := value.(string); !ok {
						return fmt.Errorf("%s.commands[%d] must be a string", path, j)
					}
				}
			}
		}
	}
	return nil
}

func objectSequence(value any, path string) ([]map[string]any, error) {
	values, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a sequence", path)
	}
	objects := make([]map[string]any, 0, len(values))
	for i, value := range values {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a mapping", path, i)
		}
		objects = append(objects, object)
	}
	return objects, nil
}

func requiredStringField(object map[string]any, key, path string) error {
	value, exists := object[key]
	if !exists {
		return fmt.Errorf("%s is required", path)
	}
	if _, ok := value.(string); !ok {
		return fmt.Errorf("%s must be a string", path)
	}
	return nil
}

func optionalStringField(object map[string]any, key, path string) error {
	value, exists := object[key]
	if !exists {
		return nil
	}
	if _, ok := value.(string); !ok {
		return fmt.Errorf("%s must be a string", path)
	}
	return nil
}

func isStringOrInteger(value any) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	default:
		return false
	}
}

func convertMain(legacy mainDocument, override string) (outputDocument, error) {
	name := chooseName(override, legacy.Name)
	if name == "" {
		return outputDocument{}, errors.New("main-v0.1 input has no name; provide --name")
	}
	if len(legacy.Steps) == 0 {
		return outputDocument{}, errors.New("steps must not be empty")
	}
	out := outputDocument{Version: "1.0", Name: name}
	for i, legacyStep := range legacy.Steps {
		path := fmt.Sprintf("steps[%d]", i)
		hasRun, hasPatch := legacyStep.Run != nil, legacyStep.Patch != nil
		if hasRun == hasPatch {
			return outputDocument{}, fmt.Errorf("%s must contain exactly one of run or patch", path)
		}
		step := outputStep{Name: strings.TrimSpace(legacyStep.Name)}
		if hasRun {
			command, err := nonblank(*legacyStep.Run, path+".run")
			if err != nil {
				return outputDocument{}, err
			}
			step.Run = []string{command}
		} else {
			patch, err := nonblank(*legacyStep.Patch, path+".patch")
			if err != nil {
				return outputDocument{}, err
			}
			step.Patch = patch
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}

func convertDevelop(legacy developDocument, override string) (outputDocument, error) {
	if legacy.Version != "0.1" {
		return outputDocument{}, fmt.Errorf("develop-v0.1 requires pxp_version %q, got %q", "0.1", legacy.Version)
	}
	if legacy.Plan == nil {
		return outputDocument{}, errors.New("plan is required")
	}
	name := chooseName(override, legacy.Plan.Task)
	if name == "" {
		return outputDocument{}, errors.New("plan.task must not be blank; provide --name")
	}
	if len(legacy.Plan.Steps) == 0 {
		return outputDocument{}, errors.New("plan.steps must not be empty")
	}
	out := outputDocument{Version: "1.0", Name: name}
	for i, legacyStep := range legacy.Plan.Steps {
		path := fmt.Sprintf("plan.steps[%d]", i)
		step := outputStep{Name: strings.TrimSpace(legacyStep.Name)}
		switch legacyStep.Type {
		case "shell":
			if legacyStep.Diff != nil {
				return outputDocument{}, fmt.Errorf("%s shell step must not contain diff", path)
			}
			hasList, hasSingle := legacyStep.Commands != nil, legacyStep.Command != nil
			if hasList == hasSingle {
				return outputDocument{}, fmt.Errorf("%s shell step must contain exactly one of commands or command", path)
			}
			if hasSingle {
				command, err := nonblank(*legacyStep.Command, path+".command")
				if err != nil {
					return outputDocument{}, err
				}
				step.Run = []string{command}
			} else {
				if len(*legacyStep.Commands) == 0 {
					return outputDocument{}, fmt.Errorf("%s.commands must not be empty", path)
				}
				for j, raw := range *legacyStep.Commands {
					command, err := nonblank(raw, fmt.Sprintf("%s.commands[%d]", path, j))
					if err != nil {
						return outputDocument{}, err
					}
					step.Run = append(step.Run, command)
				}
			}
		case "patch":
			if legacyStep.Commands != nil || legacyStep.Command != nil {
				return outputDocument{}, fmt.Errorf("%s patch step must not contain commands", path)
			}
			if legacyStep.Diff == nil {
				return outputDocument{}, fmt.Errorf("%s.diff is required", path)
			}
			patch, err := nonblank(*legacyStep.Diff, path+".diff")
			if err != nil {
				return outputDocument{}, err
			}
			step.Patch = patch
		default:
			return outputDocument{}, fmt.Errorf("%s.type must be shell or patch", path)
		}
		out.Steps = append(out.Steps, step)
	}
	return out, nil
}

func chooseName(override, legacy string) string {
	if name := strings.TrimSpace(override); name != "" {
		return name
	}
	return strings.TrimSpace(legacy)
}

func nonblank(value, path string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be blank", path)
	}
	if strings.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("%s must not contain NUL", path)
	}
	return value, nil
}

func validateOutput(out outputDocument) error {
	if out.Version != "1.0" {
		return errors.New("internal version mismatch")
	}
	if strings.TrimSpace(out.Name) == "" {
		return errors.New("name is blank")
	}
	if len(out.Name) > 1024 {
		return fmt.Errorf("name exceeds 1024 bytes")
	}
	if len(out.Steps) == 0 || len(out.Steps) > maxSteps {
		return errors.New("step count is outside v1 limits")
	}
	for i, step := range out.Steps {
		if step.Name != "" && len(step.Name) > 1024 {
			return fmt.Errorf("step %d name exceeds 1024 bytes", i)
		}
		hasRun, hasPatch := step.Run != nil, step.Patch != ""
		if hasRun == hasPatch {
			return fmt.Errorf("step %d must contain exactly one operation", i)
		}
		if len(step.Run) > maxCommands {
			return fmt.Errorf("step %d exceeds %d commands", i, maxCommands)
		}
		for _, command := range step.Run {
			if len(command) > maxCommandBytes {
				return fmt.Errorf("step %d command exceeds %d bytes", i, maxCommandBytes)
			}
		}
	}
	return nil
}
