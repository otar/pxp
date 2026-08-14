//go:build darwin || linux

package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentIntegrationSkillsEnforcePlanningExecutionBoundary(t *testing.T) {
	paths := []string{
		"integrations/claude-code/skills/pxp/SKILL.md",
		"integrations/codex/skills/pxp/SKILL.md",
	}
	for _, relative := range paths {
		t.Run(relative, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(relative)))
			if err != nil {
				t.Fatal(err)
			}
			text := string(content)
			for _, required := range []string{
				`name: pxp`,
				`pxp_version: "1.1"`,
				`git rev-parse --verify 'HEAD^{commit}'`,
				`--expect-sha256 HASH`,
				`Do not modify source files`,
				`Do not run PXP`,
				`newly approved plan`,
			} {
				if !strings.Contains(text, required) {
					t.Errorf("missing required integration contract %q", required)
				}
			}
			if strings.Contains(text, "automatic fix") && !strings.Contains(text, "Do not run PXP") {
				t.Error("integration permits adaptation without a hard planning boundary")
			}
		})
	}
}
