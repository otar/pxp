package benchmark

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Usage contains only metrics exposed by an agent's machine-readable output.
// Nil means the metric was not available and must not be inferred.
type Usage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	TotalTokens  *int64 `json:"total_tokens"`
	ModelTurns   *int64 `json:"model_turns"`
	ToolCalls    *int64 `json:"tool_calls"`
}

// ParseCodex reads `codex exec --json` JSONL. Codex reports token usage on
// turn.completed and tool operations as completed items. It does not expose
// the number of underlying model calls, so ModelTurns remains nil.
func ParseCodex(r io.Reader) (Usage, error) {
	var usage Usage
	var input, output, tools int64
	var sawEvent, sawUsage bool
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		var event struct {
			Type  string `json:"type"`
			Usage struct {
				InputTokens  int64 `json:"input_tokens"`
				OutputTokens int64 `json:"output_tokens"`
			} `json:"usage"`
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return Usage{}, fmt.Errorf("decode Codex JSONL: %w", err)
		}
		sawEvent = true
		switch event.Type {
		case "turn.completed":
			input += event.Usage.InputTokens
			output += event.Usage.OutputTokens
			sawUsage = true
		case "item.completed":
			switch event.Item.Type {
			case "command_execution", "file_change", "mcp_tool_call", "web_search":
				tools++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Usage{}, fmt.Errorf("read Codex JSONL: %w", err)
	}
	if sawEvent {
		usage.ToolCalls = int64Pointer(tools)
	}
	if sawUsage {
		usage.InputTokens = int64Pointer(input)
		usage.OutputTokens = int64Pointer(output)
		usage.TotalTokens = int64Pointer(input + output)
	}
	return usage, nil
}

// ParseClaude reads `claude -p --output-format stream-json --verbose`. Each
// assistant event is a model turn and each tool_use content block is a tool
// call. The result event contains aggregate usage for the invocation.
func ParseClaude(r io.Reader) (Usage, error) {
	var usage Usage
	var turns, tools int64
	var sawEvent bool
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	for scanner.Scan() {
		var event struct {
			Type    string           `json:"type"`
			Usage   map[string]int64 `json:"usage"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return Usage{}, fmt.Errorf("decode Claude JSONL: %w", err)
		}
		sawEvent = true
		if event.Type == "assistant" {
			turns++
			for _, content := range event.Message.Content {
				if content.Type == "tool_use" {
					tools++
				}
			}
		}
		if event.Type == "result" && event.Usage != nil {
			input := event.Usage["input_tokens"]
			output := event.Usage["output_tokens"]
			usage.InputTokens = int64Pointer(input)
			usage.OutputTokens = int64Pointer(output)
			usage.TotalTokens = int64Pointer(input + output)
		}
	}
	if err := scanner.Err(); err != nil {
		return Usage{}, fmt.Errorf("read Claude JSONL: %w", err)
	}
	if sawEvent {
		usage.ModelTurns = int64Pointer(turns)
		usage.ToolCalls = int64Pointer(tools)
	}
	return usage, nil
}

func int64Pointer(value int64) *int64 { return &value }
