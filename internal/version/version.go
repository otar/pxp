package version

import "fmt"

// Version and Commit are injected by release builds with -ldflags -X.
var (
	Version = "dev"
	Commit  = "unknown"
)

func String() string {
	return For("pxp")
}

// For returns build metadata for a named binary in the release archive.
func For(name string) string {
	return fmt.Sprintf("%s %s (%s)", name, Version, Commit)
}
