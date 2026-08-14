package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	benchmarkmetrics "github.com/otar/pxp/internal/benchmark"
)

type suite struct {
	Version string `json:"version"`
	Tasks   []task `json:"tasks"`
}

type task struct {
	Name       string   `json:"name"`
	Complexity string   `json:"complexity"`
	Fixture    string   `json:"fixture"`
	Prompt     string   `json:"prompt"`
	Verify     []string `json:"verify"`
}

type report struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	Suite         string    `json:"suite"`
	SuiteSHA256   string    `json:"suite_sha256"`
	Agent         string    `json:"agent"`
	AgentVersion  string    `json:"agent_version"`
	Model         string    `json:"model"`
	PXPVersion    string    `json:"pxp_version"`
	OS            string    `json:"os"`
	Architecture  string    `json:"architecture"`
	Results       []result  `json:"results"`
}

type result struct {
	Task               string                 `json:"task"`
	Complexity         string                 `json:"complexity"`
	Agent              string                 `json:"agent"`
	Strategy           string                 `json:"strategy"`
	Repetition         int                    `json:"repetition"`
	Usage              benchmarkmetrics.Usage `json:"usage"`
	AgentDurationMS    int64                  `json:"agent_duration_ms"`
	PlanningDurationMS *int64                 `json:"planning_duration_ms"`
	PXPDurationMS      *int64                 `json:"pxp_execution_duration_ms"`
	WallClockMS        int64                  `json:"wall_clock_duration_ms"`
	AgentExitCode      int                    `json:"agent_exit_code"`
	PXPExitCode        *int                   `json:"pxp_exit_code"`
	VerificationExit   *int                   `json:"verification_exit_code"`
	Success            bool                   `json:"success"`
	FirstPassSuccess   bool                   `json:"first_pass_success"`
	ReplanningRequired *bool                  `json:"replanning_required"`
	PlanSHA256         *string                `json:"plan_sha256"`
	Measurements       map[string]string      `json:"measurements"`
	Artifacts          string                 `json:"artifacts"`
	Error              string                 `json:"error,omitempty"`
}

type options struct {
	suitePath, suiteDir      string
	agent, agentBin, pxpBin  string
	model                    string
	outputPath, artifactsDir string
	taskFilter               string
	runs                     int
	timeout                  time.Duration
	keepWorkspaces           bool
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("pxp-bench", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var opts options
	flags.StringVar(&opts.suitePath, "suite", "benchmark/suite.json", "benchmark suite JSON")
	flags.StringVar(&opts.agent, "agent", "", "agent adapter: codex or claude")
	flags.StringVar(&opts.agentBin, "agent-bin", "", "agent executable (defaults to adapter name)")
	flags.StringVar(&opts.model, "model", "", "exact agent model name (defaults to agent configuration)")
	flags.StringVar(&opts.pxpBin, "pxp", "pxp", "PXP executable")
	flags.StringVar(&opts.outputPath, "output", "benchmark-results.json", "report JSON path")
	flags.StringVar(&opts.artifactsDir, "artifacts", "", "raw artifact directory")
	flags.StringVar(&opts.taskFilter, "task", "", "run only the named task")
	flags.IntVar(&opts.runs, "runs", 1, "repetitions per task and strategy")
	flags.DurationVar(&opts.timeout, "timeout", 30*time.Minute, "timeout for each agent, PXP, or verification command")
	flags.BoolVar(&opts.keepWorkspaces, "keep-workspaces", false, "retain temporary workspaces in artifacts")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || (opts.agent != "codex" && opts.agent != "claude") || opts.runs < 1 || opts.timeout <= 0 {
		fmt.Fprintln(stderr, "usage: pxp-bench --agent codex|claude [--suite FILE] [--runs N] [--output FILE]")
		return 2
	}
	if opts.agentBin == "" {
		opts.agentBin = opts.agent
	}
	if opts.artifactsDir == "" {
		opts.artifactsDir = opts.outputPath + ".artifacts"
	}
	absSuite, err := filepath.Abs(opts.suitePath)
	if err != nil {
		fmt.Fprintf(stderr, "resolve suite: %v\n", err)
		return 2
	}
	opts.suitePath, opts.suiteDir = absSuite, filepath.Dir(absSuite)
	document, err := loadSuite(absSuite)
	if err != nil {
		fmt.Fprintf(stderr, "load suite: %v\n", err)
		return 2
	}
	if err := os.MkdirAll(opts.artifactsDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "create artifacts: %v\n", err)
		return 70
	}

	suiteBytes, _ := os.ReadFile(absSuite)
	suiteDigest := fmt.Sprintf("%x", sha256.Sum256(suiteBytes))
	agentVersion, agentVersionErr := commandOutput(opts.timeout, opts.suiteDir, opts.agentBin, "--version")
	if agentVersionErr != nil {
		agentVersion = "unavailable: " + agentVersionErr.Error()
	}
	pxpVersion, pxpVersionErr := commandOutput(opts.timeout, opts.suiteDir, opts.pxpBin, "version")
	if pxpVersionErr != nil {
		pxpVersion = "unavailable: " + pxpVersionErr.Error()
	}
	model := opts.model
	if model == "" {
		model = "agent default; inspect raw output and agent configuration"
	}
	output := report{
		SchemaVersion: "1", GeneratedAt: time.Now().UTC(), Suite: absSuite, SuiteSHA256: suiteDigest,
		Agent: opts.agent, AgentVersion: strings.TrimSpace(agentVersion), Model: model, PXPVersion: strings.TrimSpace(pxpVersion),
		OS: runtime.GOOS, Architecture: runtime.GOARCH,
	}
	for _, currentTask := range document.Tasks {
		if opts.taskFilter != "" && opts.taskFilter != currentTask.Name {
			continue
		}
		for repetition := 1; repetition <= opts.runs; repetition++ {
			for _, strategy := range []string{"direct", "pxp"} {
				item := runOne(opts, currentTask, strategy, repetition)
				output.Results = append(output.Results, item)
				fmt.Fprintf(stderr, "%s/%s run %d: success=%t\n", currentTask.Name, strategy, repetition, item.Success)
			}
		}
	}
	if len(output.Results) == 0 {
		fmt.Fprintln(stderr, "no benchmark tasks selected")
		return 2
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode report: %v\n", err)
		return 70
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(opts.outputPath, encoded, 0o644); err != nil {
		fmt.Fprintf(stderr, "write report: %v\n", err)
		return 70
	}
	fmt.Fprintf(stdout, "benchmark report written to %s\n", opts.outputPath)
	return 0
}

func loadSuite(path string) (suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return suite{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document suite
	if err := decoder.Decode(&document); err != nil {
		return suite{}, err
	}
	if document.Version != "1" || len(document.Tasks) == 0 {
		return suite{}, errors.New("version must be \"1\" and tasks must not be empty")
	}
	seen := map[string]struct{}{}
	for index, current := range document.Tasks {
		if strings.TrimSpace(current.Name) == "" || strings.TrimSpace(current.Complexity) == "" || strings.TrimSpace(current.Prompt) == "" || len(current.Verify) == 0 {
			return suite{}, fmt.Errorf("tasks[%d] has an empty required field", index)
		}
		if filepath.IsAbs(current.Fixture) || current.Fixture == "" || strings.HasPrefix(filepath.Clean(current.Fixture), "..") {
			return suite{}, fmt.Errorf("tasks[%d].fixture must stay within the suite directory", index)
		}
		if _, duplicate := seen[current.Name]; duplicate {
			return suite{}, fmt.Errorf("duplicate task name %q", current.Name)
		}
		seen[current.Name] = struct{}{}
	}
	return document, nil
}

func runOne(opts options, current task, strategy string, repetition int) result {
	started := time.Now()
	artifactName := safeName(current.Name) + "-" + strategy + "-" + strconv.Itoa(repetition)
	artifactDir := filepath.Join(opts.artifactsDir, artifactName)
	item := result{
		Task: current.Name, Complexity: current.Complexity, Agent: opts.agent, Strategy: strategy, Repetition: repetition,
		Measurements: map[string]string{
			"agent_duration_ms": "harness monotonic clock", "wall_clock_duration_ms": "harness monotonic clock",
			"success": "verification command exit status", "first_pass_success": "single attempt; no retries",
			"input_tokens": "unavailable", "output_tokens": "unavailable", "total_tokens": "unavailable",
			"model_turns": "unavailable", "tool_calls": "unavailable",
		},
		Artifacts: artifactDir,
	}
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		item.Error = err.Error()
		return item
	}
	workspace, err := os.MkdirTemp("", "pxp-benchmark-")
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if !opts.keepWorkspaces {
		defer os.RemoveAll(workspace)
	}
	if err := copyTree(filepath.Join(opts.suiteDir, filepath.FromSlash(current.Fixture)), workspace); err != nil {
		item.Error = "prepare fixture: " + err.Error()
		return item
	}
	if err := initializeGit(opts.timeout, workspace); err != nil {
		item.Error = "initialize fixture: " + err.Error()
		return item
	}
	head, err := commandOutput(opts.timeout, workspace, "git", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		item.Error = "resolve fixture HEAD: " + err.Error()
		return item
	}
	prompt := directPrompt(current)
	if strategy == "pxp" {
		prompt = pxpPrompt(current, strings.TrimSpace(head))
	}
	_ = os.WriteFile(filepath.Join(artifactDir, "prompt.txt"), []byte(prompt), 0o644)

	agentStart := time.Now()
	agentArgs := agentArguments(opts, prompt)
	agentStdout, agentStderr, agentCode := invoke(opts.timeout, workspace, opts.agentBin, agentArgs...)
	item.AgentDurationMS = time.Since(agentStart).Milliseconds()
	item.AgentExitCode = agentCode
	_ = os.WriteFile(filepath.Join(artifactDir, "agent.stdout.jsonl"), agentStdout, 0o644)
	_ = os.WriteFile(filepath.Join(artifactDir, "agent.stderr.txt"), agentStderr, 0o644)
	usage, parseErr := parseUsage(opts.agent, agentStdout)
	if parseErr != nil {
		item.Error = parseErr.Error()
	} else {
		item.Usage = usage
		markUsageMeasurements(&item)
	}
	if strategy == "pxp" {
		planning := item.AgentDurationMS
		item.PlanningDurationMS = &planning
		item.Measurements["planning_duration_ms"] = "harness monotonic clock"
	}

	if agentCode == 0 && item.Error == "" && strategy == "pxp" {
		planPath := filepath.Join(workspace, "plan.pxp.yaml")
		planBytes, readErr := os.ReadFile(planPath)
		if readErr != nil {
			item.Error = "read generated plan: " + readErr.Error()
		} else {
			digest := fmt.Sprintf("%x", sha256.Sum256(planBytes))
			item.PlanSHA256 = &digest
			_ = os.WriteFile(filepath.Join(artifactDir, "plan.pxp.yaml"), planBytes, 0o644)
			pxpStart := time.Now()
			pxpStdout, pxpStderr, pxpCode := invoke(opts.timeout, workspace, opts.pxpBin, "run", "--expect-sha256", digest, planPath)
			pxpDuration := time.Since(pxpStart).Milliseconds()
			item.PXPDurationMS, item.PXPExitCode = &pxpDuration, &pxpCode
			item.Measurements["pxp_execution_duration_ms"] = "harness monotonic clock"
			_ = os.WriteFile(filepath.Join(artifactDir, "pxp.stdout.txt"), pxpStdout, 0o644)
			_ = os.WriteFile(filepath.Join(artifactDir, "pxp.stderr.txt"), pxpStderr, 0o644)
			if pxpCode != 0 {
				item.Error = "PXP execution failed"
			}
		}
	}

	if agentCode == 0 && item.Error == "" {
		verificationCode, log := verify(opts.timeout, workspace, current.Verify)
		item.VerificationExit = &verificationCode
		_ = os.WriteFile(filepath.Join(artifactDir, "verification.txt"), log, 0o644)
		item.Success = verificationCode == 0
		if verificationCode != 0 {
			item.Error = "verification failed"
		}
	}
	item.FirstPassSuccess = item.Success
	if strategy == "pxp" {
		replanning := !item.Success
		item.ReplanningRequired = &replanning
		item.Measurements["replanning_required"] = "PXP or verification failure after one closed plan"
	}
	if opts.keepWorkspaces {
		kept := filepath.Join(artifactDir, "workspace")
		if err := os.Rename(workspace, kept); err != nil {
			item.Error = strings.TrimSpace(item.Error + "; retain workspace: " + err.Error())
		}
	}
	item.WallClockMS = time.Since(started).Milliseconds()
	return item
}

func agentArguments(opts options, prompt string) []string {
	if opts.agent == "codex" {
		arguments := []string{"exec", "--json", "--ephemeral", "--sandbox", "workspace-write"}
		if opts.model != "" {
			arguments = append(arguments, "--model", opts.model)
		}
		return append(arguments, prompt)
	}
	arguments := []string{"-p", "--output-format", "stream-json", "--verbose", "--permission-mode", "bypassPermissions", "--no-session-persistence"}
	if opts.model != "" {
		arguments = append(arguments, "--model", opts.model)
	}
	return append(arguments, prompt)
}

func directPrompt(current task) string {
	return current.Prompt + "\n\nImplement the task directly in this repository and verify it. This benchmark permits one agent invocation and performs no follow-up or automatic retry."
}

func pxpPrompt(current task, head string) string {
	return current.Prompt + "\n\nThis is the PXP strategy. Investigate and reason, but do not modify source files or run the implementation. Create only plan.pxp.yaml: a complete PXP 1.1 plan containing all patch and run steps. Bind repository.type to git and repository.head to " + head + ". Do not execute PXP, repair failures, retry, or call an LLM from the plan. Stop after saving the closed plan."
}

func parseUsage(agent string, data []byte) (benchmarkmetrics.Usage, error) {
	if agent == "codex" {
		return benchmarkmetrics.ParseCodex(bytes.NewReader(data))
	}
	return benchmarkmetrics.ParseClaude(bytes.NewReader(data))
}

func markUsageMeasurements(item *result) {
	if item.Usage.InputTokens != nil {
		item.Measurements["input_tokens"] = "agent JSON output"
		item.Measurements["output_tokens"] = "agent JSON output"
		item.Measurements["total_tokens"] = "sum of agent-reported input and output tokens"
	}
	if item.Usage.ModelTurns != nil {
		item.Measurements["model_turns"] = "agent JSON assistant events"
	}
	if item.Usage.ToolCalls != nil {
		item.Measurements["tool_calls"] = "agent JSON tool events"
	}
}

func initializeGit(timeout time.Duration, dir string) error {
	commands := [][]string{{"init", "-q"}, {"config", "user.email", "pxp-bench@example.invalid"}, {"config", "user.name", "PXP Benchmark"}, {"add", "."}, {"commit", "-q", "-m", "benchmark fixture"}}
	for _, args := range commands {
		if _, _, code := invoke(timeout, dir, "git", args...); code != 0 {
			return fmt.Errorf("git %s exited %d", strings.Join(args, " "), code)
		}
	}
	return nil
}

func verify(timeout time.Duration, dir string, commands []string) (int, []byte) {
	var log bytes.Buffer
	for index, command := range commands {
		stdout, stderr, code := invoke(timeout, dir, "bash", "--noprofile", "--norc", "-euo", "pipefail", "-c", command)
		fmt.Fprintf(&log, "verification command %d exit=%d\n", index+1, code)
		log.Write(stdout)
		log.Write(stderr)
		if code != 0 {
			return code, log.Bytes()
		}
	}
	return 0, log.Bytes()
}

func invoke(timeout time.Duration, dir, name string, args ...string) ([]byte, []byte, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return stdout.Bytes(), append(stderr.Bytes(), []byte("command timed out\n")...), 124
	}
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), exitErr.ExitCode()
	}
	return stdout.Bytes(), append(stderr.Bytes(), []byte(err.Error()+"\n")...), 127
}

func commandOutput(timeout time.Duration, dir, name string, args ...string) (string, error) {
	stdout, stderr, code := invoke(timeout, dir, name, args...)
	if code != 0 {
		return "", fmt.Errorf("exit %d: %s", code, strings.TrimSpace(string(stderr)))
	}
	return string(stdout), nil
}

func copyTree(source, destination string) error {
	root, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !root.IsDir() {
		return errors.New("fixture is not a directory")
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink %q is not allowed", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func safeName(value string) string {
	var output strings.Builder
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			output.WriteRune(character)
		} else {
			output.WriteByte('-')
		}
	}
	return output.String()
}
