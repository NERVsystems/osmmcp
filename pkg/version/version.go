// Package version provides build metadata and version information.
package version

import (
	"fmt"
	"runtime"
)

var (
	// BuildVersion is the semantic version of the build
	BuildVersion = "0.2.0"

	// BuildCommit is the git commit hash of the build
	BuildCommit = "unknown"

	// BuildDate is the date and time of the build
	BuildDate = "unknown"

	// GoVersion is the version of Go used to build
	GoVersion = runtime.Version()
)

// String returns a formatted version string
func String() string {
	return fmt.Sprintf("osmmcp version %s (%s) built on %s with %s",
		BuildVersion, BuildCommit, BuildDate, GoVersion)
}

// Info returns a map of version information
func Info() map[string]string {
	return map[string]string{
		"version":    BuildVersion,
		"commit":     BuildCommit,
		"build_date": BuildDate,
		"go_version": GoVersion,
	}
}
