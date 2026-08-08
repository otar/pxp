package legacyconvert

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestConvertMainV01(t *testing.T) {
	input := []byte(`name: Add greeting
steps:
  - name: Say hello
    run: printf hello
  - name: Update message
    patch: |
      --- a/message.txt
      +++ b/message.txt
      @@ -1 +1 @@
      -old
      +new
`)
	doc := convertAndDecode(t, input, ProfileMainV01, "")
	if doc.Version != "1.0" || doc.Name != "Add greeting" {
		t.Fatalf("unexpected header: %#v", doc)
	}
	if got := doc.Steps[0].Run; len(got) != 1 || got[0] != "printf hello" {
		t.Fatalf("run was not normalized to an array: %#v", got)
	}
	if !strings.Contains(doc.Steps[1].Patch, "+++ b/message.txt") {
		t.Fatalf("patch was not preserved: %q", doc.Steps[1].Patch)
	}
}

func TestConvertDevelopV01NormalizesBothCommandShapesAndDropsIDs(t *testing.T) {
	input := []byte(`pxp_version: "0.1"
plan:
  task: Legacy develop plan
  steps:
    - id: 17
      name: Documented list
      type: shell
      commands:
        - printf one
        - printf two
    - id: historical-id
      name: Executor singular
      type: shell
      command: printf three
    - type: patch
      diff: |
        --- a/message.txt
        +++ b/message.txt
        @@ -1 +1 @@
        -old
        +new
`)
	converted, err := Convert(input, ProfileDevelopV01, "")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}
	for _, forbidden := range []string{"\nplan:", "\nid:", "\ntype:", "\ncommands:", "\ncommand:", "\ndiff:"} {
		if strings.Contains(string(converted), forbidden) {
			t.Errorf("converted output contains legacy field %q:\n%s", forbidden, converted)
		}
	}
	var doc decodedV1
	if err := yaml.Unmarshal(converted, &doc); err != nil {
		t.Fatalf("decode converted YAML: %v\n%s", err, converted)
	}
	if doc.Name != "Legacy develop plan" || len(doc.Steps) != 3 {
		t.Fatalf("unexpected converted plan: %#v", doc)
	}
	if got := doc.Steps[0].Run; len(got) != 2 || got[0] != "printf one" || got[1] != "printf two" {
		t.Fatalf("documented commands changed: %#v", got)
	}
	if got := doc.Steps[1].Run; len(got) != 1 || got[0] != "printf three" {
		t.Fatalf("singular command not normalized: %#v", got)
	}
}

func TestNameOverrideAndFallback(t *testing.T) {
	mainWithoutName := []byte("steps:\n  - run: printf ok\n")
	if _, err := Convert(mainWithoutName, ProfileMainV01, ""); err == nil || !strings.Contains(err.Error(), "provide --name") {
		t.Fatalf("missing main name error = %v", err)
	}
	doc := convertAndDecode(t, mainWithoutName, ProfileMainV01, "Override")
	if doc.Name != "Override" {
		t.Fatalf("override name = %q", doc.Name)
	}

	develop := []byte("pxp_version: \"0.1\"\nplan:\n  task: Old\n  steps:\n    - type: shell\n      command: printf ok\n")
	doc = convertAndDecode(t, develop, ProfileDevelopV01, "New")
	if doc.Name != "New" {
		t.Fatalf("develop override name = %q", doc.Name)
	}
}

func TestConvertRejectsAmbiguousOrMalformedLegacyInput(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		input   string
		want    string
	}{
		{"unknown profile", "auto", "name: x\nsteps: [{run: echo x}]\n", "unsupported source profile"},
		{"wrong develop version", ProfileDevelopV01, "pxp_version: \"1.0\"\nplan: {task: x, steps: [{type: shell, command: echo}]}", "requires pxp_version"},
		{"unknown root field", ProfileMainV01, "name: x\nextra: true\nsteps: [{run: echo}]\n", "unknown field"},
		{"duplicate field", ProfileMainV01, "name: x\nname: y\nsteps: [{run: echo}]\n", "already defined"},
		{"anchor", ProfileMainV01, "name: x\nsteps: &s\n  - run: echo\n", "anchors and aliases"},
		{"multiple docs", ProfileMainV01, "name: x\nsteps: [{run: echo}]\n---\nname: y\nsteps: [{run: echo}]\n", "multiple YAML documents"},
		{"main both operations", ProfileMainV01, "name: x\nsteps: [{run: echo, patch: diff}]\n", "exactly one"},
		{"develop both command forms", ProfileDevelopV01, "pxp_version: \"0.1\"\nplan:\n  task: x\n  steps:\n    - type: shell\n      command: echo one\n      commands: [echo two]\n", "exactly one"},
		{"develop shell with diff", ProfileDevelopV01, "pxp_version: \"0.1\"\nplan:\n  task: x\n  steps:\n    - type: shell\n      command: echo\n      diff: no\n", "must not contain diff"},
		{"develop unknown type", ProfileDevelopV01, "pxp_version: \"0.1\"\nplan:\n  task: x\n  steps: [{type: copy, command: echo}]\n", "must be shell or patch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Convert([]byte(test.input), test.profile, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Convert() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestConvertRejectsInvalidUTF8AndOversize(t *testing.T) {
	if _, err := Convert([]byte{0xff}, ProfileMainV01, "x"); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
	if _, err := Convert(make([]byte, maxInputBytes+1), ProfileMainV01, "x"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestConvertRejectsScalarCoercionAndOutputLimitViolations(t *testing.T) {
	tests := []struct {
		profile string
		input   string
	}{
		{ProfileMainV01, "name: 42\nsteps: [{run: echo}]\n"},
		{ProfileMainV01, "name: x\nsteps: [{run: 42}]\n"},
		{ProfileDevelopV01, "pxp_version: 0.1\nplan: {task: x, steps: [{type: shell, command: echo}]}\n"},
		{ProfileDevelopV01, "pxp_version: \"0.1\"\nplan: {task: 42, steps: [{type: shell, command: echo}]}\n"},
		{ProfileDevelopV01, "pxp_version: \"0.1\"\nplan: {task: x, steps: [{type: shell, commands: [42]}]}\n"},
		{ProfileDevelopV01, "pxp_version: \"0.1\"\nplan: {task: x, steps: [{id: true, type: shell, command: echo}]}\n"},
	}
	for _, test := range tests {
		if _, err := Convert([]byte(test.input), test.profile, ""); err == nil {
			t.Errorf("Convert(%s, %q) succeeded", test.profile, test.input)
		}
	}
	longName := strings.Repeat("x", 1025)
	if _, err := Convert([]byte("steps: [{run: echo}]\n"), ProfileMainV01, longName); err == nil {
		t.Fatal("over-limit output name succeeded")
	}
}

type decodedV1 struct {
	Version string `yaml:"pxp_version"`
	Name    string `yaml:"name"`
	Steps   []struct {
		Name  string   `yaml:"name"`
		Run   []string `yaml:"run"`
		Patch string   `yaml:"patch"`
	} `yaml:"steps"`
}

func convertAndDecode(t *testing.T, input []byte, profile, override string) decodedV1 {
	t.Helper()
	converted, err := Convert(input, profile, override)
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}
	var doc decodedV1
	if err := yaml.Unmarshal(converted, &doc); err != nil {
		t.Fatalf("decode converted YAML: %v\n%s", err, converted)
	}
	return doc
}
