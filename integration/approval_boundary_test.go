//go:build darwin || linux

package integration_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpectedPlanSHA256ApprovalBoundary(t *testing.T) {
	t.Run("matching hash executes", func(t *testing.T) {
		repo := newRepository(t)
		input := runPlan("touch hash-match-ran")
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
		result := invoke(t, repo, []string{"run", "--expect-sha256", digest, "-"}, input, nil)
		if result.code != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertExists(t, filepath.Join(repo, "hash-match-ran"))
	})

	t.Run("mismatching hash stops before a step", func(t *testing.T) {
		repo := newRepository(t)
		input := runPlan("touch hash-mismatch-ran")
		result := invoke(t, repo, []string{"run", "--expect-sha256", strings.Repeat("0", 64), "-"}, input, nil)
		if result.code != 4 || !strings.Contains(result.stderr, "plan SHA-256 mismatch") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertAbsent(t, filepath.Join(repo, "hash-mismatch-ran"))
	})

	t.Run("modified file after approval is rejected", func(t *testing.T) {
		repo := newRepository(t)
		approved := runPlan("touch approved-command-ran")
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(approved)))
		planPath := filepath.Join(repo, "plan.pxp.yaml")
		writeFile(t, planPath, approved)
		writeFile(t, planPath, runPlan("touch modified-command-ran"))

		result := invoke(t, repo, []string{"run", "--expect-sha256", digest, planPath}, "", nil)
		if result.code != 4 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertAbsent(t, filepath.Join(repo, "approved-command-ran"))
		assertAbsent(t, filepath.Join(repo, "modified-command-ran"))
	})
}

func TestGitHeadBinding(t *testing.T) {
	t.Run("matching head executes", func(t *testing.T) {
		repo := newRepository(t)
		input := boundRunPlan(t, repo, "touch head-match-ran")
		result := invoke(t, repo, []string{"run", "-"}, input, nil)
		if result.code != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertExists(t, filepath.Join(repo, "head-match-ran"))
	})

	t.Run("mismatching head stops before a step", func(t *testing.T) {
		repo := newRepository(t)
		input := boundRunPlan(t, repo, "touch head-mismatch-ran")
		writeFile(t, filepath.Join(repo, "later.txt"), "later\n")
		runGit(t, repo, "add", "later.txt")
		runGit(t, repo, "commit", "-q", "-m", "advance head")

		result := invoke(t, repo, []string{"run", "-"}, input, nil)
		if result.code != 4 || !strings.Contains(result.stderr, "git HEAD mismatch") {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertAbsent(t, filepath.Join(repo, "head-mismatch-ran"))
	})

	t.Run("v1.0 remains unbound", func(t *testing.T) {
		repo := newRepository(t)
		input := runPlan("touch legacy-unbound-ran")
		writeFile(t, filepath.Join(repo, "later.txt"), "later\n")
		runGit(t, repo, "add", "later.txt")
		runGit(t, repo, "commit", "-q", "-m", "advance head")

		result := invoke(t, repo, []string{"run", "-"}, input, nil)
		if result.code != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertExists(t, filepath.Join(repo, "legacy-unbound-ran"))
	})
}

func TestExpectedSHA256UsageValidation(t *testing.T) {
	repo := newRepository(t)
	for name, args := range map[string][]string{
		"missing value": {"run", "--expect-sha256"},
		"invalid value": {"run", "--expect-sha256", "ABC", "-"},
		"duplicate":     {"run", "--expect-sha256", strings.Repeat("0", 64), "--expect-sha256", strings.Repeat("1", 64), "-"},
	} {
		t.Run(name, func(t *testing.T) {
			result := invoke(t, repo, args, runPlan("touch never"), nil)
			if result.code != 2 {
				t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
		})
	}
}

func boundRunPlan(t *testing.T, repo, command string) string {
	t.Helper()
	head := strings.TrimSpace(runGitOutput(t, repo, "rev-parse", "--verify", "HEAD^{commit}"))
	return "pxp_version: \"1.1\"\nname: Bound test\nrepository:\n  type: git\n  head: " + head + "\nsteps:\n  - run:\n      - " + yamlSingleQuote(command) + "\n"
}
