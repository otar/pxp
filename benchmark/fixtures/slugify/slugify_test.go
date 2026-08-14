package slugify

import "testing"

func TestSlugify(t *testing.T) {
	for input, want := range map[string]string{
		"Hello, World!":    "hello-world",
		"  many---spaces ": "many-spaces",
		"Crème brûlée 42":  "crème-brûlée-42",
		"already-clean":    "already-clean",
		"---":              "",
	} {
		if got := Slugify(input); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", input, got, want)
		}
	}
}
