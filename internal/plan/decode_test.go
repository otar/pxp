package plan

import (
	"fmt"
	"strings"
	"testing"
)

const validRunPlan = `pxp_version: "1.0"
name: Test plan
steps:
  - name: Verify
    run:
      - echo ok
`

func TestDecodeValidRunPlan(t *testing.T) {
	document, err := Decode([]byte(validRunPlan))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if document.Name != "Test plan" || len(document.Steps) != 1 || len(document.Steps[0].Run) != 1 {
		t.Fatalf("Decode() = %#v", document)
	}
}

func TestDecodeStrictYAML(t *testing.T) {
	tests := map[string]string{
		"unknown field":      strings.Replace(validRunPlan, "name: Test plan", "name: Test plan\nunknown: true", 1),
		"duplicate key":      strings.Replace(validRunPlan, "name: Test plan", "name: First\nname: Second", 1),
		"multiple documents": validRunPlan + "---\npxp_version: \"1.0\"\nname: Other\nsteps:\n  - run: [true]\n",
		"anchor":             strings.Replace(validRunPlan, "name: Test plan", "name: &plan_name Test plan", 1),
		"numeric version":    strings.Replace(validRunPlan, `pxp_version: "1.0"`, "pxp_version: 1.0", 1),
		"scalar run":         strings.Replace(validRunPlan, "run:\n      - echo ok", "run: echo ok", 1),
		"both operations":    strings.Replace(validRunPlan, "      - echo ok", "      - echo ok\n    patch: |\n      --- a/a.txt\n      +++ b/a.txt\n      @@ -1 +1 @@\n      -a\n      +b", 1),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(input)); err == nil {
				t.Fatal("Decode() unexpectedly succeeded")
			}
		})
	}
}

func TestPlanLimits(t *testing.T) {
	validCommand := "echo ok"
	patch := "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-a\n+b\n"
	tests := map[string]*Plan{
		"top-level name": {
			PXPVersion: ProtocolVersion,
			Name:       strings.Repeat("n", MaxNameBytes+1),
			Steps:      []Step{{Run: []string{validCommand}}},
		},
		"step name": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      []Step{{Name: stringPointer(strings.Repeat("n", MaxNameBytes+1)), Run: []string{validCommand}}},
		},
		"step count": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      make([]Step, MaxSteps+1),
		},
		"command count": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      []Step{{Run: repeatCommands(validCommand, MaxRunCommands+1)}},
		},
		"command bytes": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      []Step{{Run: []string{strings.Repeat("x", MaxCommandBytes+1)}}},
		},
		"command NUL": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      []Step{{Run: []string{"echo\x00bad"}}},
		},
		"patch NUL": {
			PXPVersion: ProtocolVersion,
			Name:       "plan",
			Steps:      []Step{{Patch: stringPointer(patch + "\x00")}},
		},
	}

	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if err := document.validate(); err == nil {
				t.Fatal("validate() unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeAcceptsBOMAndHashesOriginalBytes(t *testing.T) {
	withBOM := append([]byte{0xef, 0xbb, 0xbf}, []byte(validRunPlan)...)
	document, err := Decode(withBOM)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if document.SHA256 == "" {
		t.Fatal("Decode() did not set SHA256")
	}
}

func TestDocumentSizeLimit(t *testing.T) {
	if _, err := Decode(make([]byte, MaxDocumentBytes+1)); err == nil {
		t.Fatal("Decode() unexpectedly accepted oversized input")
	}
}

func TestValidatePatchPolicy(t *testing.T) {
	validModify := `diff --git a/file.txt b/file.txt
index 7898192..6178079 100644
--- a/file.txt
+++ b/file.txt
@@ -1 +1 @@
-a
+b
`
	validCreate := `diff --git a/new.sh b/new.sh
new file mode 100755
index 0000000..8b2fe54
--- /dev/null
+++ b/new.sh
@@ -0,0 +1 @@
+echo ok
`
	validDelete := `diff --git a/old.txt b/old.txt
deleted file mode 100644
index 7898192..0000000
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-a
`

	for name, diff := range map[string]string{
		"modify 100644": validModify,
		"create 100755": validCreate,
		"delete 100644": validDelete,
	} {
		t.Run("allow "+name, func(t *testing.T) {
			if err := validatePatch(diff); err != nil {
				t.Fatalf("validatePatch() error = %v", err)
			}
		})
	}

	rejected := map[string]string{
		"rename": `diff --git a/old.txt b/new.txt
similarity index 100%
rename from old.txt
rename to new.txt
`,
		"copy": `diff --git a/old.txt b/new.txt
similarity index 100%
copy from old.txt
copy to new.txt
`,
		"pure mode": `diff --git a/file.sh b/file.sh
old mode 100644
new mode 100755
`,
		"content and mode": `diff --git a/file.sh b/file.sh
old mode 100644
new mode 100755
--- a/file.sh
+++ b/file.sh
@@ -1 +1 @@
-old
+new
`,
		"symlink": `diff --git a/link b/link
new file mode 120000
index 0000000..1de5659
--- /dev/null
+++ b/link
@@ -0,0 +1 @@
+target
`,
		"gitlink": `diff --git a/module b/module
new file mode 160000
index 0000000..0123456
--- /dev/null
+++ b/module
@@ -0,0 +1 @@
+Subproject commit 0123456789012345678901234567890123456789
`,
		"escape": `--- a/../outside.txt
+++ b/../outside.txt
@@ -1 +1 @@
-a
+b
`,
		"Git metadata": `--- a/.git/config
+++ b/.git/config
@@ -1 +1 @@
-a
+b
`,
		"dot component": `--- a/./file.txt
+++ b/./file.txt
@@ -1 +1 @@
-a
+b
`,
	}
	for name, diff := range rejected {
		t.Run("reject "+name, func(t *testing.T) {
			if err := validatePatch(diff); err == nil {
				t.Fatal("validatePatch() unexpectedly succeeded")
			}
		})
	}
}

func repeatCommands(command string, count int) []string {
	commands := make([]string, count)
	for i := range commands {
		commands[i] = command
	}
	return commands
}

func stringPointer(value string) *string { return &value }

func ExampleDecode() {
	document, err := Decode([]byte(validRunPlan))
	fmt.Println(document.Name, err)
	// Output: Test plan <nil>
}
