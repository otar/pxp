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
				`pxp run plan.pxp.yaml`,
				`Do not modify source files`,
				`Do not run PXP`,
				`newly reviewed plan`,
			} {
				if !strings.Contains(text, required) {
					t.Errorf("missing required integration contract %q", required)
				}
			}
			for _, forbidden := range []string{`--expect-sha256`, `sha256sum`, `shasum -a 256`} {
				if strings.Contains(text, forbidden) {
					t.Errorf("default integration contract contains optional hash workflow %q", forbidden)
				}
			}
			if strings.Contains(text, "automatic fix") && !strings.Contains(text, "Do not run PXP") {
				t.Error("integration permits adaptation without a hard planning boundary")
			}
		})
	}
}
