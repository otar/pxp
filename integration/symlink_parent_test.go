//go:build darwin || linux

package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditTrackedSymlinkParentWithAllowDirty(t *testing.T) {
	repo := newRepository(t)
	outsideDir := t.TempDir()
	link := filepath.Join(repo, "escape")
	if err := os.Symlink(outsideDir, link); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "escape")
	runGit(t, repo, "commit", "-q", "-m", "track symlink parent")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}

	diff := "diff --git a/escape/created.txt b/escape/created.txt\n" +
		"new file mode 100644\n--- /dev/null\n+++ b/escape/created.txt\n" +
		"@@ -0,0 +1 @@\n+escaped\n"
	result := invoke(t, repo, []string{"run", "--allow-dirty", "-"}, patchPlan(diff), nil)
	if result.code != 3 {
		t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, filepath.Join(outsideDir, "created.txt"))
}

func TestAuditPriorRunCannotIntroduceSymlinkParent(t *testing.T) {
	repo := newRepository(t)
	outsideDir := t.TempDir()
	diff := "diff --git a/escape/created.txt b/escape/created.txt\n" +
		"new file mode 100644\n--- /dev/null\n+++ b/escape/created.txt\n" +
		"@@ -0,0 +1 @@\n+escaped\n"
	plan := fmt.Sprintf(
		"pxp_version: \"1.0\"\nname: Test\nsteps:\n"+
			"  - run:\n      - %s\n"+
			"  - patch: |\n      %s\n",
		yamlSingleQuote("ln -s "+shellQuote(outsideDir)+" escape"),
		strings.ReplaceAll(diff, "\n", "\n      "),
	)
	result := invoke(t, repo, []string{"run", "-"}, plan, nil)
	if result.code != 5 {
		t.Fatalf("exit=%d want=5 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, filepath.Join(outsideDir, "created.txt"))
}
