package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Repository is the resolved Git worktree in which a plan executes.
type Repository struct {
	Root     string
	LockPath string
	gitPath  string
}

// Open resolves the Git worktree containing startDir.
func Open(ctx context.Context, startDir string) (*Repository, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git is required: %w", err)
	}
	if err := checkGitVersion(ctx, gitPath, startDir); err != nil {
		return nil, err
	}

	root, err := gitOutput(ctx, gitPath, startDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("resolve Git worktree: %w", err)
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("resolve Git worktree: Git returned an empty root")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Git worktree root: %w", err)
	}

	lockPath, err := gitOutput(ctx, gitPath, root, "rev-parse", "--git-path", "pxp.lock")
	if err != nil {
		return nil, fmt.Errorf("resolve Git lock path: %w", err)
	}
	lockPath = strings.TrimSpace(lockPath)
	if !filepath.IsAbs(lockPath) {
		lockPath = filepath.Join(root, lockPath)
	}

	return &Repository{
		Root:     filepath.Clean(root),
		LockPath: filepath.Clean(lockPath),
		gitPath:  gitPath,
	}, nil
}

// CheckClean requires a clean tracked worktree and index. Untracked and
// ignored files do not make the repository dirty.
func (r *Repository) CheckClean(ctx context.Context) error {
	out, err := r.git(ctx, nil, "status", "--porcelain=v1", "--untracked-files=no")
	if err != nil {
		return fmt.Errorf("inspect tracked state: %w", err)
	}
	if len(bytes.TrimSpace(out)) != 0 {
		return fmt.Errorf("tracked files are modified; commit/stash them or pass --allow-dirty")
	}
	return nil
}

// CheckPatch asks Git whether diff applies to the current worktree.
func (r *Repository) CheckPatch(ctx context.Context, diff string) error {
	if err := r.CheckPatchPolicy(ctx, diff); err != nil {
		return err
	}
	_, err := r.git(ctx, strings.NewReader(diff), "apply", "--check", "--whitespace=nowarn", "-")
	if err != nil {
		return fmt.Errorf("patch preflight failed: %w", err)
	}
	return nil
}

// ApplyPatch applies a patch that has passed CheckPatch.
func (r *Repository) ApplyPatch(ctx context.Context, diff string) error {
	if err := r.CheckPatchPolicy(ctx, diff); err != nil {
		return err
	}
	_, err := r.git(ctx, strings.NewReader(diff), "apply", "--whitespace=nowarn", "-")
	if err != nil {
		return fmt.Errorf("apply patch: %w", err)
	}
	return nil
}

// CheckPatchPolicy validates repository-state-dependent path policy before any
// command or patch mutates the worktree.
func (r *Repository) CheckPatchPolicy(ctx context.Context, diff string) error {
	paths, err := r.patchTargetPaths(ctx, diff)
	if err != nil {
		return fmt.Errorf("inspect patch paths: %w", err)
	}
	seen := map[string]struct{}{}
	uniquePaths := make([]string, 0, len(paths))
	for _, name := range paths {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		uniquePaths = append(uniquePaths, name)
		if err := r.checkFilesystemPath(name); err != nil {
			return err
		}
	}
	return r.checkTrackedPathPrefixes(ctx, uniquePaths)
}

func (r *Repository) checkTrackedPathPrefixes(ctx context.Context, paths []string) error {
	prefixes := map[string]struct{}{}
	for _, name := range paths {
		parts := strings.Split(name, "/")
		for i := range parts {
			prefixes[strings.Join(parts[:i+1], "/")] = struct{}{}
		}
	}

	orderedPrefixes := make([]string, 0, len(prefixes))
	for prefix := range prefixes {
		orderedPrefixes = append(orderedPrefixes, prefix)
	}
	sort.Strings(orderedPrefixes)
	args := []string{"ls-files", "-s", "-z", "--"}
	for _, prefix := range orderedPrefixes {
		args = append(args, ":(top,literal)"+prefix)
	}
	out, err := r.git(ctx, nil, args...)
	if err != nil {
		return fmt.Errorf("inspect tracked path modes: %w", err)
	}

	trackedModes := map[string][]uint64{}
	for _, record := range bytes.Split(out, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		tab := bytes.IndexByte(record, '\t')
		if tab < 0 {
			return fmt.Errorf("inspect tracked path modes: malformed Git output")
		}
		metadata := strings.Fields(string(record[:tab]))
		if len(metadata) == 0 {
			return fmt.Errorf("inspect tracked path modes: malformed Git output")
		}
		mode, err := strconv.ParseUint(metadata[0], 8, 32)
		if err != nil {
			return fmt.Errorf("inspect tracked path modes: malformed mode %q", metadata[0])
		}
		name := string(record[tab+1:])
		if _, needed := prefixes[name]; needed {
			trackedModes[name] = append(trackedModes[name], mode)
		}
	}

	for _, name := range paths {
		parts := strings.Split(name, "/")
		for i := range parts {
			prefix := strings.Join(parts[:i+1], "/")
			modes := trackedModes[prefix]
			if len(modes) == 0 {
				continue
			}
			if i < len(parts)-1 {
				return fmt.Errorf("patch path %q traverses tracked non-directory %q", name, prefix)
			}
			for _, mode := range modes {
				if mode != 0o100644 && mode != 0o100755 {
					return fmt.Errorf("patch path %q is not a tracked regular file", name)
				}
			}
		}
	}
	return nil
}

func (r *Repository) patchTargetPaths(ctx context.Context, diff string) ([]string, error) {
	out, err := r.git(ctx, strings.NewReader(diff), "apply", "--numstat", "-z", "-")
	if err != nil {
		return nil, err
	}
	records := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(records))
	for _, record := range records {
		if len(record) == 0 {
			continue
		}
		firstTab := bytes.IndexByte(record, '\t')
		if firstTab < 0 {
			return nil, fmt.Errorf("malformed Git numstat output")
		}
		secondRelative := bytes.IndexByte(record[firstTab+1:], '\t')
		if secondRelative < 0 {
			return nil, fmt.Errorf("malformed Git numstat output")
		}
		name := string(record[firstTab+1+secondRelative+1:])
		if name == "" {
			return nil, fmt.Errorf("git returned an empty patch path")
		}
		paths = append(paths, name)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("git returned no patch paths")
	}
	return paths, nil
}

func (r *Repository) checkFilesystemPath(name string) error {
	current := r.Root
	parts := strings.Split(filepath.FromSlash(name), string(filepath.Separator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect patch path %q: %w", name, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("patch path %q traverses a symbolic link", name)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return fmt.Errorf("patch path %q traverses a non-directory", name)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("patch path %q is not a regular file", name)
		}
	}
	return nil
}

func checkGitVersion(ctx context.Context, gitPath, dir string) error {
	out, err := gitOutput(ctx, gitPath, dir, "--version")
	if err != nil {
		return fmt.Errorf("inspect Git version: %w", err)
	}
	var major, minor int
	if _, err := fmt.Sscanf(out, "git version %d.%d", &major, &minor); err != nil {
		return fmt.Errorf("parse Git version from %q", strings.TrimSpace(out))
	}
	if major < 2 || major == 2 && minor < 24 {
		return fmt.Errorf("git 2.24 or newer is required (found %d.%d)", major, minor)
	}
	return nil
}

func (r *Repository) git(ctx context.Context, stdin *strings.Reader, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.gitPath, args...)
	cmd.Dir = r.Root
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return out, nil
}

func gitOutput(ctx context.Context, gitPath, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, gitPath, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return "", err
		}
		return "", fmt.Errorf("%s", detail)
	}
	return string(out), nil
}
