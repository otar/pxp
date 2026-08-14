package plan

import (
	"strings"
	"testing"
)

const testGitHead = "0123456789abcdef0123456789abcdef01234567"

func TestDecodeRepositoryBindingVersions(t *testing.T) {
	bound := `pxp_version: "1.1"
name: Bound plan
repository:
  type: git
  head: ` + testGitHead + `
steps:
  - run: [echo ok]
`
	document, err := Decode([]byte(bound))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if document.Repository == nil || document.Repository.Head != testGitHead {
		t.Fatalf("Decode() repository = %#v", document.Repository)
	}

	unbound, err := Decode([]byte(validRunPlan))
	if err != nil {
		t.Fatalf("Decode(v1.0) error = %v", err)
	}
	if unbound.Repository != nil {
		t.Fatalf("Decode(v1.0) repository = %#v, want nil", unbound.Repository)
	}
}

func TestDecodeRejectsMalformedRepositoryBinding(t *testing.T) {
	base := `pxp_version: "1.1"
name: Bound plan
repository:
  type: git
  head: ` + testGitHead + `
steps:
  - run: [echo ok]
`
	tests := map[string]string{
		"missing repository": strings.Replace(base, "repository:\n  type: git\n  head: "+testGitHead+"\n", "", 1),
		"repository in v1.0": strings.Replace(base, `pxp_version: "1.1"`, `pxp_version: "1.0"`, 1),
		"missing type":       strings.Replace(base, "  type: git\n", "", 1),
		"unknown type":       strings.Replace(base, "  type: git", "  type: svn", 1),
		"short head":         strings.Replace(base, testGitHead, "01234567", 1),
		"uppercase head":     strings.Replace(base, testGitHead, strings.ToUpper(testGitHead), 1),
		"non-hex head":       strings.Replace(base, testGitHead, strings.Repeat("z", 40), 1),
		"unknown field":      strings.Replace(base, "  head: "+testGitHead, "  head: "+testGitHead+"\n  branch: main", 1),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(input)); err == nil {
				t.Fatal("Decode() unexpectedly succeeded")
			}
		})
	}
}
