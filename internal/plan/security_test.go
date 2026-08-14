package plan

import (
	"fmt"
	"strings"
	"testing"
)

const minimalPlan = `pxp_version: "1.0"
name: Test plan
steps:
  - run:
      - printf ok
`

func TestDecodeAcceptsSimpleYAMLRepresentations(t *testing.T) {
	inputs := map[string][]byte{
		"LF":       []byte(minimalPlan),
		"CRLF":     []byte(strings.ReplaceAll(minimalPlan, "\n", "\r\n")),
		"UTF8 BOM": append([]byte{0xef, 0xbb, 0xbf}, []byte(minimalPlan)...),
		"comments and reordered step keys": []byte(`pxp_version: "1.0"
name: Test plan # ordinary comments are allowed
steps:
  - run: [printf ok]
    name: Inline command
`),
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(input); err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
		})
	}
}

func TestDecodeRejectsAmbiguousOrComplexYAML(t *testing.T) {
	tests := map[string]string{
		"empty":                        "",
		"legacy main":                  "name: x\nsteps: [{run: [echo]}]\n",
		"legacy develop":               "pxp_version: \"0.1\"\nplan: {task: x, steps: []}\n",
		"numeric version":              "pxp_version: 1.0\nname: x\nsteps: [{run: [echo]}]\n",
		"null version":                 "pxp_version: null\nname: x\nsteps: [{run: [echo]}]\n",
		"numeric name":                 "pxp_version: \"1.0\"\nname: 42\nsteps: [{run: [echo]}]\n",
		"boolean name":                 "pxp_version: \"1.0\"\nname: true\nsteps: [{run: [echo]}]\n",
		"null name":                    "pxp_version: \"1.0\"\nname: null\nsteps: [{run: [echo]}]\n",
		"numeric command":              "pxp_version: \"1.0\"\nname: x\nsteps: [{run: [42]}]\n",
		"null command":                 "pxp_version: \"1.0\"\nname: x\nsteps: [{run: [null]}]\n",
		"scalar run":                   "pxp_version: \"1.0\"\nname: x\nsteps: [{run: echo}]\n",
		"sequence patch":               "pxp_version: \"1.0\"\nname: x\nsteps: [{patch: [x]}]\n",
		"null patch":                   "pxp_version: \"1.0\"\nname: x\nsteps: [{patch: null}]\n",
		"unknown root":                 "pxp_version: \"1.0\"\nname: x\nunknown: y\nsteps: [{run: [echo]}]\n",
		"unknown step":                 "pxp_version: \"1.0\"\nname: x\nsteps: [{run: [echo], unknown: y}]\n",
		"duplicate root key":           "pxp_version: \"1.0\"\nname: x\nname: y\nsteps: [{run: [echo]}]\n",
		"duplicate operation key":      "pxp_version: \"1.0\"\nname: x\nsteps:\n  - run: [echo]\n    run: [echo again]\n",
		"anchor":                       "pxp_version: \"1.0\"\nname: x\nsteps: &s [{run: [echo]}]\n",
		"alias":                        "pxp_version: \"1.0\"\nname: x\nbase: &s [{run: [echo]}]\nsteps: *s\n",
		"explicit tag":                 "pxp_version: \"1.0\"\nname: !!str x\nsteps: [{run: [echo]}]\n",
		"directive":                    "%YAML 1.2\n---\npxp_version: \"1.0\"\nname: x\nsteps: [{run: [echo]}]\n",
		"multiple documents":           minimalPlan + "---\n" + minimalPlan,
		"both operations":              "pxp_version: \"1.0\"\nname: x\nsteps: [{run: [echo], patch: diff}]\n",
		"neither operation":            "pxp_version: \"1.0\"\nname: x\nsteps: [{name: nope}]\n",
		"empty run":                    "pxp_version: \"1.0\"\nname: x\nsteps: [{run: []}]\n",
		"blank command":                "pxp_version: \"1.0\"\nname: x\nsteps: [{run: ['   ']}]\n",
		"blank plan name":              "pxp_version: \"1.0\"\nname: '   '\nsteps: [{run: [echo]}]\n",
		"blank optional step name":     "pxp_version: \"1.0\"\nname: x\nsteps: [{name: '   ', run: [echo]}]\n",
		"missing 1.1 repository":       "pxp_version: \"1.1\"\nname: x\nsteps: [{run: [echo]}]\n",
		"unsupported protocol version": "pxp_version: \"2.0\"\nname: x\nsteps: [{run: [echo]}]\n",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(input)); err == nil {
				t.Fatal("Decode() succeeded, want rejection")
			}
		})
	}
}

func TestDecodeEnforcesByteAndCountLimits(t *testing.T) {
	validName := strings.Repeat("é", MaxNameBytes/2)
	if _, err := Decode([]byte(strings.Replace(minimalPlan, "Test plan", validName, 1))); err != nil {
		t.Fatalf("exactly MaxNameBytes should pass: %v", err)
	}
	tooLongName := validName + "é"
	if _, err := Decode([]byte(strings.Replace(minimalPlan, "Test plan", tooLongName, 1))); err == nil {
		t.Fatal("name over MaxNameBytes succeeded")
	}

	command := strings.Repeat("x", MaxCommandBytes)
	if _, err := Decode([]byte(strings.Replace(minimalPlan, "printf ok", command, 1))); err != nil {
		t.Fatalf("exactly MaxCommandBytes should pass: %v", err)
	}
	if _, err := Decode([]byte(strings.Replace(minimalPlan, "printf ok", command+"x", 1))); err == nil {
		t.Fatal("command over MaxCommandBytes succeeded")
	}

	var commands strings.Builder
	commands.WriteString("pxp_version: \"1.0\"\nname: x\nsteps:\n  - run:\n")
	for i := 0; i < MaxRunCommands+1; i++ {
		commands.WriteString("      - printf x\n")
	}
	if _, err := Decode([]byte(commands.String())); err == nil {
		t.Fatal("run over MaxRunCommands succeeded")
	}

	var steps strings.Builder
	steps.WriteString("pxp_version: \"1.0\"\nname: x\nsteps:\n")
	for i := 0; i < MaxSteps+1; i++ {
		steps.WriteString("  - run: [printf x]\n")
	}
	if _, err := Decode([]byte(steps.String())); err == nil {
		t.Fatal("plan over MaxSteps succeeded")
	}

	if _, err := Read(strings.NewReader(strings.Repeat("x", MaxDocumentBytes+1))); err == nil {
		t.Fatal("document over MaxDocumentBytes succeeded")
	}
}

func TestDecodeRejectsInvalidUTF8AndNUL(t *testing.T) {
	if _, err := Decode([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 succeeded")
	}
	input := strings.Replace(minimalPlan, "printf ok", "printf "+string([]byte{0}), 1)
	if _, err := Decode([]byte(input)); err == nil {
		t.Fatal("NUL succeeded")
	}
}

func TestPatchPolicy(t *testing.T) {
	valid := map[string]string{
		"modify": `diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-old
+new
`,
		"create regular file": `diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+new
`,
		"delete regular file": `diff --git a/old.txt b/old.txt
deleted file mode 100644
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-old
`,
	}
	for name, diff := range valid {
		t.Run("valid "+name, func(t *testing.T) {
			if _, err := Decode(planWithPatch(diff)); err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
		})
	}

	invalid := map[string]string{
		"parent traversal": `diff --git a/../outside.txt b/../outside.txt
--- a/../outside.txt
+++ b/../outside.txt
@@ -1 +1 @@
-old
+new
`,
		"dot component": `diff --git a/dir/./file.txt b/dir/./file.txt
--- a/dir/./file.txt
+++ b/dir/./file.txt
@@ -1 +1 @@
-old
+new
`,
		"empty component": `diff --git a/dir//file.txt b/dir//file.txt
--- a/dir//file.txt
+++ b/dir//file.txt
@@ -1 +1 @@
-old
+new
`,
		"git metadata": `diff --git a/.git/config b/.git/config
--- a/.git/config
+++ b/.git/config
@@ -1 +1 @@
-old
+new
`,
		"backslash": `diff --git "a/dir\\file.txt" "b/dir\\file.txt"
--- "a/dir\\file.txt"
+++ "b/dir\\file.txt"
@@ -1 +1 @@
-old
+new
`,
		"symlink mode": `diff --git a/link b/link
new file mode 120000
--- /dev/null
+++ b/link
@@ -0,0 +1 @@
+outside
`,
		"gitlink mode": `diff --git a/submodule b/submodule
new file mode 160000
--- /dev/null
+++ b/submodule
@@ -0,0 +1 @@
+Subproject commit 0123456789012345678901234567890123456789
`,
		"rename metadata": `diff --git a/old.txt b/new.txt
similarity index 100%
rename from old.txt
rename to new.txt
`,
		"binary patch": `diff --git a/file.bin b/file.bin
GIT binary patch
literal 0
HcmV?d00001
`,
		"mode only": `diff --git a/file.txt b/file.txt
old mode 100644
new mode 100755
`,
		"preamble": "review me first\n" + valid["modify"],
	}
	for name, diff := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			if _, err := Decode(planWithPatch(diff)); err == nil {
				t.Fatal("Decode() succeeded, want patch-policy rejection")
			}
		})
	}
}

func planWithPatch(diff string) []byte {
	indented := "      " + strings.ReplaceAll(diff, "\n", "\n      ")
	return []byte(fmt.Sprintf("pxp_version: \"1.0\"\nname: Patch test\nsteps:\n  - patch: |\n%s\n", indented))
}
