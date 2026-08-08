//go:build darwin || linux

package integration_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallerChecksumFailureNeverReplacesDestination(t *testing.T) {
	fixture := newInstallerFixture(t)
	destination := filepath.Join(fixture.binDir, "pxp")
	writeFile(t, destination, "existing destination\n")
	writeFile(t, filepath.Join(fixture.releaseDir, "SHA256SUMS"), strings.Repeat("0", 64)+"  "+fixture.archiveName+"\n")

	result := fixture.run(t, "--version", "v1.0.0", "--bin-dir", fixture.binDir, "--force")
	if result.code == 0 || !strings.Contains(result.stderr, "checksum") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if got := readFile(t, destination); got != "existing destination\n" {
		t.Fatalf("destination changed after checksum failure: %q", got)
	}
	assertNoInstallTemps(t, fixture.binDir)
}

func TestInstallerVerifiedInstallIsAtomicAndExecutable(t *testing.T) {
	fixture := newInstallerFixture(t)
	result := fixture.run(t, "--version", "v1.0.0", "--bin-dir", fixture.binDir)
	if result.code != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	destination := filepath.Join(fixture.binDir, "pxp")
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode=%#o want=0755", info.Mode().Perm())
	}
	cmd := exec.Command(destination, "version")
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "pxp 1.0.0") {
		t.Fatalf("installed smoke test: err=%v output=%q", err, output)
	}
	assertNoInstallTemps(t, fixture.binDir)
}

func TestInstallerRequiresForceWithoutExecutingExistingDestination(t *testing.T) {
	fixture := newInstallerFixture(t)
	destination := filepath.Join(fixture.binDir, "pxp")
	marker := filepath.Join(fixture.root, "existing-was-executed")
	original := fmt.Sprintf("#!/usr/bin/env bash\nprintf 'executed\\n' > %q\nprintf 'pxp 0.0.0 (untrusted)\\n'\n", marker)
	writeExecutable(t, destination, original)

	result := fixture.run(t, "--version", "v1.0.0", "--bin-dir", fixture.binDir)
	if result.code == 0 || !strings.Contains(result.stderr, "--force") {
		t.Fatalf("without --force: exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if _, err := os.Stat(marker); err == nil || !os.IsNotExist(err) {
		t.Fatalf("existing destination was executed: %v", err)
	}
	if got := readFile(t, destination); got != original {
		t.Fatalf("existing destination was replaced: %q", got)
	}

	result = fixture.run(t, "--version", "v1.0.0", "--bin-dir", fixture.binDir, "--force")
	if result.code != 0 {
		t.Fatalf("with --force: exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	cmd := exec.Command(destination, "version")
	if output, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(output), "pxp 1.0.0") {
		t.Fatalf("forced install smoke test: err=%v output=%q", err, output)
	}
}

func TestInstallerRejectsUnexpectedArchiveWithoutReplacing(t *testing.T) {
	fixture := newInstallerFixture(t)
	destination := filepath.Join(fixture.binDir, "pxp")
	writeFile(t, destination, "existing destination\n")
	writeFile(t, filepath.Join(fixture.archiveSource, "unexpected"), "surprise\n")
	fixture.rebuildArchive(t, "LICENSE", "README.md", "pxp", "pxp-convert", "unexpected")
	fixture.writeChecksum(t)

	result := fixture.run(t, "--version", "v1.0.0", "--bin-dir", fixture.binDir, "--force")
	if result.code == 0 || !strings.Contains(result.stderr, "unexpected contents") {
		t.Fatalf("exit=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
	if got := readFile(t, destination); got != "existing destination\n" {
		t.Fatalf("destination changed after archive rejection: %q", got)
	}
}

type installerFixture struct {
	root, releaseDir, archiveSource, fakeBin, binDir, archiveName string
}

func newInstallerFixture(t *testing.T) *installerFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &installerFixture{
		root:          root,
		releaseDir:    filepath.Join(root, "release"),
		archiveSource: filepath.Join(root, "archive-source"),
		fakeBin:       filepath.Join(root, "fake-bin"),
		binDir:        filepath.Join(root, "install-bin"),
		archiveName:   fmt.Sprintf("pxp_1.0.0_%s_%s.tar.gz", releaseOS(), releaseArch()),
	}
	for _, dir := range []string{fixture.releaseDir, fixture.archiveSource, fixture.fakeBin, fixture.binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable(t, filepath.Join(fixture.archiveSource, "pxp"), "#!/usr/bin/env bash\nif [[ \"${1:-}\" == version ]]; then printf 'pxp 1.0.0 (test)\\n'; else exit 2; fi\n")
	writeExecutable(t, filepath.Join(fixture.archiveSource, "pxp-convert"), "#!/usr/bin/env bash\nexit 0\n")
	writeFile(t, filepath.Join(fixture.archiveSource, "LICENSE"), "test license\n")
	writeFile(t, filepath.Join(fixture.archiveSource, "README.md"), "test readme\n")
	fixture.rebuildArchive(t, "LICENSE", "README.md", "pxp", "pxp-convert")
	fixture.writeChecksum(t)

	fakeCurl := `#!/usr/bin/env bash
set -euo pipefail
output=""
url="${!#}"
while (($#)); do
  case "$1" in
    -o|-[A-Za-z]*o)
      shift
      output="$1"
      ;;
  esac
  shift
done
[[ -n "$output" ]]
cp "$PXP_FAKE_RELEASE/$(basename "$url")" "$output"
`
	writeExecutable(t, filepath.Join(fixture.fakeBin, "curl"), fakeCurl)
	return fixture
}

func (f *installerFixture) rebuildArchive(t *testing.T, members ...string) {
	t.Helper()
	args := []string{"-czf", filepath.Join(f.releaseDir, f.archiveName), "-C", f.archiveSource}
	args = append(args, members...)
	cmd := exec.Command("tar", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create archive: %v\n%s", err, output)
	}
}

func (f *installerFixture) writeChecksum(t *testing.T) {
	t.Helper()
	archive, err := os.ReadFile(filepath.Join(f.releaseDir, f.archiveName))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	writeFile(t, filepath.Join(f.releaseDir, "SHA256SUMS"), fmt.Sprintf("%x  %s\n", digest, f.archiveName))
}

func (f *installerFixture) run(t *testing.T, args ...string) commandResult {
	t.Helper()
	installer := filepath.Join(moduleRoot, "install.sh")
	cmdArgs := append([]string{installer}, args...)
	cmd := exec.Command("bash", cmdArgs...)
	cmd.Dir = f.root
	cmd.Env = append(
		filteredEnvironment(os.Environ(), "PATH", "PXP_FAKE_RELEASE", "PXP_VERSION", "PXP_INSTALL_DIR", "PXP_BIN_DIR"),
		"PATH="+f.fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PXP_FAKE_RELEASE="+f.releaseDir,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return commandResult{code: exitCode(err), stdout: stdout.String(), stderr: stderr.String()}
}

func releaseOS() string {
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	return "linux"
}

func releaseArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "amd64"
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertNoInstallTemps(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".pxp.install.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary install files remain: %v", matches)
	}
}
