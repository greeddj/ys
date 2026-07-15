// Package helpers builds the human-readable version string from the build-time
// metadata injected via ldflags, falling back to the latest published release.
package helpers

import (
	"fmt"
	"os"
	"runtime"
)

const (
	defaultVersion = "latest"
	defaultBuilder = "go"
)

// Version returns the formatted version string for the application.
func Version(version, commit, date, builtBy string) string {
	if version == "" {
		version = defaultVersion
	}

	if builtBy == "" {
		builtBy = defaultBuilder
	}

	switch {
	case date != "" && commit != "":
		return fmt.Sprintf("%s (commit %s, built by %s @ %s) // %s", version, commit, builtBy, date, runtime.Version())
	case date == "" && commit != "":
		return fmt.Sprintf("%s (commit %s, built by %s) // %s", version, commit, builtBy, runtime.Version())
	case date != "" && commit == "":
		return fmt.Sprintf("%s (built by %s @ %s) // %s", version, builtBy, date, runtime.Version())
	default:
		return fmt.Sprintf("%s (built by %s) // %s", version, builtBy, runtime.Version())
	}
}

// WantColor decides whether to colorize output. An explicit flag wins; otherwise
// color is used only on a terminal and never when NO_COLOR is set.
func WantColor(force, disable bool) bool {
	switch {
	case disable:
		return false
	case force:
		return true
	case os.Getenv("NO_COLOR") != "":
		return false
	default:
		return isTerminal(os.Stdout)
	}
}

// isTerminal reports whether f is a character device, i.e. an interactive
// terminal rather than a pipe or regular file.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
