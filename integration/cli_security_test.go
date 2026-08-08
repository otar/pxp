//go:build darwin || linux

package integration_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	pxpBinary       string
	converterBinary string
	moduleRoot      string
)

func TestMain(m *testing.M) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(source))
	moduleRoot = root
	buildDir, err := os.MkdirTemp("", "pxp-integration-build-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(70)
	}
	pxpBinary = filepath.Join(buildDir, "pxp")
	converterBinary = filepath.Join(buildDir, "pxp-convert")
	goTool := os.Getenv("GO_EXECUTABLE")
	if goTool == "" {
		goTool = "go"
	}
	build := exec.Command(goTool, "build", "-o", pxpBinary, "./cmd/pxp")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build pxp: %v\n%s", err, output)
		_ = os.RemoveAll(buildDir)
		os.Exit(70)
	}
	build = exec.Command(goTool, "build", "-o", converterBinary, "./cmd/pxp-convert")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build pxp-convert: %v\n%s", err, output)
		_ = os.RemoveAll(buildDir)
		os.Exit(70)
	}
	code := m.Run()
	_ = os.RemoveAll(buildDir)
	os.Exit(code)
}

func TestCLIUsageAndVersion(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want int
	}{
		{"help", []string{"help"}, 0},
		{"long help", []string{"--help"}, 0},
		{"version", []string{"version"}, 0},
		{"long version", []string{"--version"}, 0},
		{"missing command", nil, 2},
		{"unknown command", []string{"inspect"}, 2},
		{"missing plan", []string{"run"}, 2},
		{"extra plan", []string{"run", "one", "two"}, 2},
		{"unknown flag", []string{"run", "--unsafe", "-"}, 2},
		{"duplicate allow dirty", []string{"run", "--allow-dirty", "--allow-dirty", "-"}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := invoke(t, t.TempDir(), test.args, "", nil)
			if result.code != test.want {
				t.Fatalf("exit=%d want=%d stdout=%q stderr=%q", result.code, test.want, result.stdout, result.stderr)
			}
		})
	}
}

func TestWholePlanIsValidatedBeforeMutation(t *testing.T) {
	repo := newRepository(t)
	invalid := `pxp_version: "1.0"
name: Late invalid step
steps:
  - run:
      - touch must-not-exist
  - run: []
`
	result := invoke(t, repo, []string{"run", "-"}, invalid, nil)
	if result.code != 3 {
		t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, filepath.Join(repo, "must-not-exist"))
}

func TestExecutorRejectsBothLegacyFormats(t *testing.T) {
	repo := newRepository(t)
	legacy := []string{
		"name: main legacy\nsteps:\n  - run: printf old\n",
		"pxp_version: \"0.1\"\nplan:\n  task: develop legacy\n  steps:\n    - type: shell\n      command: printf old\n",
	}
	for i, input := range legacy {
		result := invoke(t, repo, []string{"run", "-"}, input, nil)
		if result.code != 3 {
			t.Errorf("legacy[%d] exit=%d want=3", i, result.code)
		}
	}
}

func TestTrackedDirtyPolicyAndUntrackedAllowance(t *testing.T) {
	t.Run("untracked is allowed", func(t *testing.T) {
		repo := newRepository(t)
		writeFile(t, filepath.Join(repo, "already-untracked"), "x")
		result := invoke(t, repo, []string{"run", "-"}, runPlan("touch ran"), nil)
		if result.code != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertExists(t, filepath.Join(repo, "ran"))
	})

	for _, state := range []string{"unstaged", "staged"} {
		t.Run(state, func(t *testing.T) {
			repo := newRepository(t)
			writeFile(t, filepath.Join(repo, "tracked.txt"), "dirty\n")
			if state == "staged" {
				runGit(t, repo, "add", "tracked.txt")
			}
			result := invoke(t, repo, []string{"run", "-"}, runPlan("touch blocked"), nil)
			if result.code != 4 {
				t.Fatalf("exit=%d want=4 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
			assertAbsent(t, filepath.Join(repo, "blocked"))

			result = invoke(t, repo, []string{"run", "--allow-dirty", "-"}, runPlan("touch allowed"), nil)
			if result.code != 0 {
				t.Fatalf("allow-dirty exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
			assertExists(t, filepath.Join(repo, "allowed"))
		})
	}
}

func TestUnmergedTrackedStateIsRejected(t *testing.T) {
	repo := newRepository(t)
	baseBranch := strings.TrimSpace(runGitOutput(t, repo, "branch", "--show-current"))
	runGit(t, repo, "checkout", "-q", "-b", "conflicting-branch")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "branch version\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-q", "-m", "branch change")
	runGit(t, repo, "checkout", "-q", baseBranch)
	writeFile(t, filepath.Join(repo, "tracked.txt"), "base version\n")
	runGit(t, repo, "add", "tracked.txt")
	runGit(t, repo, "commit", "-q", "-m", "base change")
	merge := exec.Command("git", "merge", "--no-edit", "conflicting-branch")
	merge.Dir = repo
	if err := merge.Run(); err == nil {
		t.Fatal("fixture merge unexpectedly succeeded")
	}

	result := invoke(t, repo, []string{"run", "-"}, runPlan("touch blocked"), nil)
	if result.code != 4 {
		t.Fatalf("exit=%d want=4 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, filepath.Join(repo, "blocked"))
}

func TestCommandsRunAtRootInFreshStrictBash(t *testing.T) {
	repo := newRepository(t)
	subdir := filepath.Join(repo, "nested")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	input := `pxp_version: "1.0"
name: Bash behavior
steps:
  - run:
      - PXP_LOCAL_STATE=set
      - test -z "${PXP_LOCAL_STATE+x}"
      - test "$PWD" = "$(git rev-parse --show-toplevel)"
      - printf root > root-marker
`
	result := invoke(t, subdir, []string{"run", "-"}, input, nil)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertExists(t, filepath.Join(repo, "root-marker"))
	assertAbsent(t, filepath.Join(subdir, "root-marker"))

	for _, command := range []string{"false | true", `: "$PXP_DEFINITELY_UNSET"`} {
		repo := newRepository(t)
		result := invoke(t, repo, []string{"run", "-"}, runPlan(command), nil)
		if result.code != 5 {
			t.Errorf("strict command %q exit=%d want=5", command, result.code)
		}
	}
}

func TestAmbientBashStartupVariablesAreRemoved(t *testing.T) {
	repo := newRepository(t)
	evil := filepath.Join(t.TempDir(), "bash-env")
	writeFile(t, evil, "touch \"$PWD/injected-by-bash-env\"\n")
	env := []string{"BASH_ENV=" + evil, "ENV=" + evil, "CDPATH=" + t.TempDir()}
	input := runPlan(`test -z "${BASH_ENV+x}" && test -z "${ENV+x}" && test -z "${CDPATH+x}"`)
	result := invoke(t, repo, []string{"run", "-"}, input, env)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertAbsent(t, filepath.Join(repo, "injected-by-bash-env"))
}

func TestChildCannotConsumePlanOrCallerStdin(t *testing.T) {
	repo := newRepository(t)
	planPath := filepath.Join(t.TempDir(), "plan.yaml")
	writeFile(t, planPath, runPlan(`if read -r secret; then exit 9; fi`))
	result := invoke(t, repo, []string{"run", planPath}, "caller-secret\n", nil)
	if result.code != 0 {
		t.Fatalf("file input exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}

	result = invoke(t, repo, []string{"run", "-"}, runPlan(`if read -r secret; then exit 9; fi`), nil)
	if result.code != 0 {
		t.Fatalf("stdin plan exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
}

func TestCommandFailureStopsRemainingCommands(t *testing.T) {
	repo := newRepository(t)
	input := `pxp_version: "1.0"
name: Stop on failure
steps:
  - run:
      - touch before
      - 'false'
      - touch after
  - run:
      - touch later-step
`
	result := invoke(t, repo, []string{"run", "-"}, input, nil)
	if result.code != 5 {
		t.Fatalf("exit=%d want=5 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	assertExists(t, filepath.Join(repo, "before"))
	assertAbsent(t, filepath.Join(repo, "after"))
	assertAbsent(t, filepath.Join(repo, "later-step"))
}

func TestPatchAppliesIndentedDeletionAndCreatesRegularFile(t *testing.T) {
	repo := newRepository(t)
	diff := `diff --git a/tracked.txt b/tracked.txt
--- a/tracked.txt
+++ b/tracked.txt
@@ -1,4 +1,4 @@
 alpha
-  old
+  new
 gamma
 delta
diff --git a/created.txt b/created.txt
new file mode 100644
--- /dev/null
+++ b/created.txt
@@ -0,0 +1 @@
+created
`
	result := invoke(t, repo, []string{"run", "-"}, patchPlan(diff), nil)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if got := readFile(t, filepath.Join(repo, "tracked.txt")); got != "alpha\n  new\ngamma\ndelta\n" {
		t.Fatalf("tracked.txt=%q", got)
	}
	if got := readFile(t, filepath.Join(repo, "created.txt")); got != "created\n" {
		t.Fatalf("created.txt=%q", got)
	}
}

func TestPatchPreflightPreventsPartialApplication(t *testing.T) {
	repo := newRepository(t)
	diff := `diff --git a/tracked.txt b/tracked.txt
--- a/tracked.txt
+++ b/tracked.txt
@@ -1,4 +1,4 @@
 alpha
-  old
+  new
 gamma
 delta
diff --git a/second.txt b/second.txt
--- a/second.txt
+++ b/second.txt
@@ -1 +1 @@
-not-the-current-content
+changed
`
	result := invoke(t, repo, []string{"run", "-"}, patchPlan(diff), nil)
	if result.code != 5 {
		t.Fatalf("exit=%d want=5 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if got := readFile(t, filepath.Join(repo, "tracked.txt")); got != "alpha\n  old\ngamma\ndelta\n" {
		t.Fatalf("first file partially changed: %q", got)
	}
	assertAbsent(t, filepath.Join(repo, "second.txt.rej"))
}

func TestPatchPathPolicyRejectsBeforeGit(t *testing.T) {
	tests := map[string]string{
		"traversal": `diff --git a/../outside.txt b/../outside.txt
--- a/../outside.txt
+++ b/../outside.txt
@@ -1 +1 @@
-outside
+escaped
`,
		"git metadata": `diff --git a/.git/config b/.git/config
--- a/.git/config
+++ b/.git/config
@@ -1 +1 @@
-x
+y
`,
		"dot component": `diff --git a/dir/./file.txt b/dir/./file.txt
--- a/dir/./file.txt
+++ b/dir/./file.txt
@@ -1 +1 @@
-x
+y
`,
		"empty component": `diff --git a/dir//file.txt b/dir//file.txt
--- a/dir//file.txt
+++ b/dir//file.txt
@@ -1 +1 @@
-x
+y
`,
		"symlink": `diff --git a/link b/link
new file mode 120000
--- /dev/null
+++ b/link
@@ -0,0 +1 @@
+outside
`,
	}
	for name, diff := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newRepository(t)
			outside := filepath.Join(filepath.Dir(repo), "outside.txt")
			writeFile(t, outside, "outside\n")
			result := invoke(t, repo, []string{"run", "-"}, patchPlan(diff), nil)
			if result.code != 3 {
				t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
			}
			if got := readFile(t, outside); got != "outside\n" {
				t.Fatalf("outside file changed: %q", got)
			}
		})
	}
}

func TestPatchRejectsExistingSymlinkAndSymlinkParent(t *testing.T) {
	t.Run("existing tracked symlink", func(t *testing.T) {
		repo := newRepository(t)
		outside := filepath.Join(t.TempDir(), "outside.txt")
		writeFile(t, outside, "outside\n")
		link := filepath.Join(repo, "link")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		runGit(t, repo, "add", "link")
		runGit(t, repo, "commit", "-q", "-m", "track symlink")
		diff := fmt.Sprintf("diff --git a/link b/link\n--- a/link\n+++ b/link\n@@ -1 +1 @@\n-%s\n\\ No newline at end of file\n+replacement\n\\ No newline at end of file\n", outside)
		result := invoke(t, repo, []string{"run", "-"}, patchPlan(diff), nil)
		if result.code != 3 {
			t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		if target, err := os.Readlink(link); err != nil || target != outside {
			t.Fatalf("symlink changed: target=%q err=%v", target, err)
		}
		if got := readFile(t, outside); got != "outside\n" {
			t.Fatalf("outside file changed: %q", got)
		}
	})

	t.Run("untracked symlink parent", func(t *testing.T) {
		repo := newRepository(t)
		outsideDir := t.TempDir()
		if err := os.Symlink(outsideDir, filepath.Join(repo, "escape")); err != nil {
			t.Fatal(err)
		}
		diff := `diff --git a/escape/created.txt b/escape/created.txt
new file mode 100644
--- /dev/null
+++ b/escape/created.txt
@@ -0,0 +1 @@
+escaped
`
		result := invoke(t, repo, []string{"run", "-"}, patchPlan(diff), nil)
		if result.code != 3 {
			t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertAbsent(t, filepath.Join(outsideDir, "created.txt"))
	})

	t.Run("deleted tracked symlink parent with allow-dirty", func(t *testing.T) {
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
		diff := `diff --git a/escape/created.txt b/escape/created.txt
new file mode 100644
--- /dev/null
+++ b/escape/created.txt
@@ -0,0 +1 @@
+escaped
`
		result := invoke(t, repo, []string{"run", "--allow-dirty", "-"}, patchPlan(diff), nil)
		if result.code != 3 {
			t.Fatalf("exit=%d want=3 stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
		}
		assertAbsent(t, filepath.Join(repo, "escape", "created.txt"))
		assertAbsent(t, filepath.Join(outsideDir, "created.txt"))
	})
}

func TestRepositoryLockIsExclusiveAndReleased(t *testing.T) {
	repo := newRepository(t)
	coord := t.TempDir()
	ready := filepath.Join(coord, "ready")
	release := filepath.Join(coord, "release")
	blockingPlan := runPlan(fmt.Sprintf("touch %s; while [ ! -e %s ]; do sleep 0.05; done", shellQuote(ready), shellQuote(release)))
	planPath := filepath.Join(coord, "blocking.yaml")
	writeFile(t, planPath, blockingPlan)

	first := exec.Command(pxpBinary, "run", planPath)
	first.Dir = repo
	var firstOutput bytes.Buffer
	first.Stdout, first.Stderr = &firstOutput, &firstOutput
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, ready, 5*time.Second)

	second := invoke(t, repo, []string{"run", "-"}, runPlan("printf second"), nil)
	if second.code != 4 {
		t.Fatalf("contender exit=%d want=4 stdout=%q stderr=%q", second.code, second.stdout, second.stderr)
	}
	writeFile(t, release, "release")
	if err := waitCommand(first, 5*time.Second); err != nil {
		t.Fatalf("first run: %v output=%q", err, firstOutput.String())
	}

	third := invoke(t, repo, []string{"run", "-"}, runPlan("printf third"), nil)
	if third.code != 0 {
		t.Fatalf("post-release exit=%d stdout=%q stderr=%q", third.code, third.stdout, third.stderr)
	}
}

func TestSignalsReturnConventionalCodesAndReleaseLock(t *testing.T) {
	for _, test := range []struct {
		name   string
		signal os.Signal
		code   int
	}{
		{"SIGINT", os.Interrupt, 130},
		{"SIGTERM", syscall.SIGTERM, 143},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepository(t)
			coord := t.TempDir()
			ready := filepath.Join(coord, "ready")
			orphanMarker := filepath.Join(coord, "child-survived")
			planPath := filepath.Join(t.TempDir(), "blocking.yaml")
			command := fmt.Sprintf("touch %s; (sleep 1; touch %s) & while :; do sleep 1; done", shellQuote(ready), shellQuote(orphanMarker))
			writeFile(t, planPath, runPlan(command))
			cmd := exec.Command(pxpBinary, "run", planPath)
			cmd.Dir = repo
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waitForFile(t, ready, 5*time.Second)
			if err := cmd.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}
			err := waitCommand(cmd, 5*time.Second)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.code {
				t.Fatalf("wait error=%v exit=%d want=%d", err, exitCode(err), test.code)
			}
			time.Sleep(1200 * time.Millisecond)
			assertAbsent(t, orphanMarker)
			result := invoke(t, repo, []string{"run", "-"}, runPlan("printf unlocked"), nil)
			if result.code != 0 {
				t.Fatalf("lock not released: exit=%d stderr=%q", result.code, result.stderr)
			}
		})
	}
}

func TestSignalsInterruptBlockedPlanStdin(t *testing.T) {
	for _, test := range []struct {
		name   string
		signal os.Signal
		code   int
	}{
		{"SIGINT", os.Interrupt, 130},
		{"SIGTERM", syscall.SIGTERM, 143},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepository(t)
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()

			cmd := exec.Command(pxpBinary, "run", "-")
			cmd.Dir = repo
			cmd.Stdin = reader
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				reader.Close()
				t.Fatal(err)
			}
			// The child owns its duplicated descriptor after Start. Keeping only
			// the writer open ensures its plan read remains blocked until signaled.
			if err := reader.Close(); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal(err)
			}
			time.Sleep(250 * time.Millisecond)
			if err := cmd.Process.Signal(test.signal); err != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal(err)
			}

			err = waitCommand(cmd, 5*time.Second)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.code {
				t.Fatalf("wait error=%v exit=%d want=%d stdout=%q stderr=%q", err, exitCode(err), test.code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "error[interrupted]") {
				t.Fatalf("stderr missing interruption category: %q", stderr.String())
			}
			if strings.Contains(stderr.String(), "error[plan]") {
				t.Fatalf("signal-induced read failure was misclassified: %q", stderr.String())
			}
		})
	}
}

func TestLifecycleUsesStderrDigestAndDoesNotEchoCommands(t *testing.T) {
	repo := newRepository(t)
	secret := "PXP_TEST_SECRET_7ed93"
	input := runPlan("printf child-output; printf child-error >&2; : " + secret)
	result := invoke(t, repo, []string{"run", "-"}, input, nil)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
	if result.stdout != "child-output" {
		t.Errorf("stdout contains lifecycle text: %q", result.stdout)
	}
	for _, want := range []string{"Test", digest, "child-error"} {
		if !strings.Contains(result.stderr, want) {
			t.Errorf("stderr missing %q: %q", want, result.stderr)
		}
	}
	if strings.Contains(result.stdout+result.stderr, secret) {
		t.Errorf("executor echoed command secret: stdout=%q stderr=%q", result.stdout, result.stderr)
	}
}

type commandResult struct {
	code           int
	stdout, stderr string
}

func invoke(t *testing.T, cwd string, args []string, stdin string, extraEnv []string) commandResult {
	t.Helper()
	cmd := exec.Command(pxpBinary, args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(filteredEnvironment(os.Environ(), "BASH_ENV", "ENV", "CDPATH"), extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return commandResult{code: exitCode(err), stdout: stdout.String(), stderr: stderr.String()}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func filteredEnvironment(env []string, names ...string) []string {
	blocked := make(map[string]struct{}, len(names))
	for _, name := range names {
		blocked[name] = struct{}{}
	}
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		name, _, _ := strings.Cut(item, "=")
		if _, ok := blocked[name]; !ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func newRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.email", "pxp-test@example.invalid")
	runGit(t, repo, "config", "user.name", "PXP Test")
	writeFile(t, filepath.Join(repo, "tracked.txt"), "alpha\n  old\ngamma\ndelta\n")
	writeFile(t, filepath.Join(repo, "second.txt"), "second\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "fixture")
	return repo
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func runPlan(command string) string {
	return "pxp_version: \"1.0\"\nname: Test\nsteps:\n  - run:\n      - " + yamlSingleQuote(command) + "\n"
}

func patchPlan(diff string) string {
	return "pxp_version: \"1.0\"\nname: Test\nsteps:\n  - patch: |\n      " + strings.ReplaceAll(diff, "\n", "\n      ") + "\n"
}

func yamlSingleQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, stat error=%v", path, err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func waitCommand(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return errors.New("command timed out")
	}
}
