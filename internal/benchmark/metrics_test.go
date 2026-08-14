package benchmark

import (
	"strings"
	"testing"
)

func TestParseCodex(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"1","type":"command_execution"}}`,
		`{"type":"item.completed","item":{"id":"2","type":"file_change"}}`,
		`{"type":"item.completed","item":{"id":"3","type":"agent_message"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":100,"cached_input_tokens":80,"output_tokens":25,"reasoning_output_tokens":5}}`,
	}, "\n")
	usage, err := ParseCodex(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 100 || usage.OutputTokens == nil || *usage.OutputTokens != 25 || usage.TotalTokens == nil || *usage.TotalTokens != 125 {
		t.Fatalf("tokens = %#v", usage)
	}
	if usage.ToolCalls == nil || *usage.ToolCalls != 2 {
		t.Fatalf("tool calls = %#v", usage.ToolCalls)
	}
	if usage.ModelTurns != nil {
		t.Fatalf("model turns = %v, want unavailable", *usage.ModelTurns)
	}
}

func TestParseClaude(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"assistant","message":{"content":[{"type":"text"},{"type":"tool_use"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use"},{"type":"tool_use"}]}}`,
		`{"type":"result","usage":{"input_tokens":80,"output_tokens":20,"cache_read_input_tokens":50}}`,
	}, "\n")
	usage, err := ParseClaude(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if usage.TotalTokens == nil || *usage.TotalTokens != 100 {
		t.Fatalf("tokens = %#v", usage)
	}
	if usage.ModelTurns == nil || *usage.ModelTurns != 2 || usage.ToolCalls == nil || *usage.ToolCalls != 3 {
		t.Fatalf("counts = %#v", usage)
	}
}

func TestParsersRejectMalformedJSONL(t *testing.T) {
	if _, err := ParseCodex(strings.NewReader("not json\n")); err == nil {
		t.Fatal("ParseCodex() unexpectedly succeeded")
	}
	if _, err := ParseClaude(strings.NewReader("not json\n")); err == nil {
		t.Fatal("ParseClaude() unexpectedly succeeded")
	}
}

func TestParsersLeaveUnexposedMetricsUnavailable(t *testing.T) {
	codex, err := ParseCodex(strings.NewReader(""))
	if err != nil || codex.ToolCalls != nil || codex.TotalTokens != nil {
		t.Fatalf("empty Codex usage = %#v, %v", codex, err)
	}
	claude, err := ParseClaude(strings.NewReader(""))
	if err != nil || claude.ToolCalls != nil || claude.ModelTurns != nil || claude.TotalTokens != nil {
		t.Fatalf("empty Claude usage = %#v, %v", claude, err)
	}
}
