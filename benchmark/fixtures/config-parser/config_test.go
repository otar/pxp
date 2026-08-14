package configparser

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := "# comment\nPORT = 8080\nGREETING=\"hello\\nworld\"\nPATH=\"c:\\\\tmp\"\nUNICODE=ქართული\n"
	got, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"PORT": "8080", "GREETING": "hello\nworld", "PATH": `c:\tmp`, "UNICODE": "ქართული"}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func TestParseRejectsInvalidInputWithLine(t *testing.T) {
	tests := []string{
		"GOOD=x\nGOOD=y\n",
		"1BAD=x\n",
		"GOOD\n",
		"GOOD=\"unterminated\n",
		"GOOD=\"ok\" trailing\n",
		"GOOD=\"bad\\t\"\n",
	}
	for _, input := range tests {
		if _, err := Parse(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), "line ") {
			t.Errorf("Parse(%q) error = %v, want line-numbered failure", input, err)
		}
	}
}
