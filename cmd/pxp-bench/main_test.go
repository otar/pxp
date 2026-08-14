//go:build darwin || linux

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunProducesDirectAndPXPResults(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join(root, "fixture")
	if err := os.Mkdir(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(fixture, "value.txt"), "old\n", 0o644)

	agent := filepath.Join(root, "fake-codex")
	writeTestFile(t, agent, `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == --version ]]; then printf 'fake codex 1.0\n'; exit 0; fi
prompt="${!#}"
if [[ "$prompt" == *"This is the PXP strategy"* ]]; then
  head=$(git rev-parse --verify 'HEAD^{commit}')
  printf 'pxp_version: "1.1"\nname: benchmark\nrepository:\n  type: git\n  head: %s\nsteps:\n  - run:\n      - printf new\\n > value.txt\n' "$head" > plan.pxp.yaml
else
  printf 'new\n' > value.txt
fi
printf '%s\n' '{"type":"item.completed","item":{"type":"file_change"}}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}'
`, 0o755)

	pxp := filepath.Join(root, "fake-pxp")
	writeTestFile(t, pxp, `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == version ]]; then printf 'fake pxp 1.0\n'; exit 0; fi
[[ "$1" == run && "$2" == --expect-sha256 ]]
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$4" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$4" | awk '{print $1}')
fi
[[ "$actual" == "$3" ]]
printf 'new\n' > value.txt
`, 0o755)

	suitePath := filepath.Join(root, "suite.json")
	writeTestFile(t, suitePath, `{
  "version": "1",
  "tasks": [{
    "name": "change-value",
    "complexity": "small",
    "fixture": "fixture",
    "prompt": "Change value.txt from old to new.",
    "verify": ["test \"$(cat value.txt)\" = new"]
  }]
}
`, 0o644)
	outputPath := filepath.Join(root, "results.json")
	artifactsPath := filepath.Join(root, "artifacts")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--agent", "codex",
		"--agent-bin", agent,
		"--pxp", pxp,
		"--suite", suitePath,
		"--output", outputPath,
		"--artifacts", artifactsPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run()=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Results) != 2 || !got.Results[0].Success || !got.Results[1].Success {
		t.Fatalf("results = %#v", got.Results)
	}
	if got.Results[0].Strategy != "direct" || got.Results[1].Strategy != "pxp" || got.Results[1].PlanSHA256 == nil || got.Results[1].PXPDurationMS == nil {
		t.Fatalf("results = %#v", got.Results)
	}
}

func TestLoadSuiteRejectsFixtureTraversal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.json")
	writeTestFile(t, path, `{"version":"1","tasks":[{"name":"x","complexity":"small","fixture":"../outside","prompt":"x","verify":["true"]}]}`, 0o644)
	if _, err := loadSuite(path); err == nil {
		t.Fatal("loadSuite() unexpectedly succeeded")
	}
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}
