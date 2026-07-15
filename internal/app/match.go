package app

import (
	"fmt"
	"regexp"
	"strings"
)

// matcher reports whether a dotted path (and its last segment) matches the
// user-supplied key.
type matcher func(dottedPath, lastSegment string) bool

// buildMatcher returns the matcher selected by the mode flags. When both modes
// are set, regexp takes precedence.
func buildMatcher(key string, regexMode, pathMode bool) (matcher, error) {
	switch {
	case regexMode:
		re, err := regexp.Compile(key)
		if err != nil {
			return nil, fmt.Errorf("bad regexp %q: %w", key, err)
		}
		return func(dotted, _ string) bool { return re.MatchString(dotted) }, nil
	case pathMode:
		return func(dotted, _ string) bool {
			return dotted == key || strings.HasSuffix(dotted, "."+key)
		}, nil
	default:
		return func(_, last string) bool { return last == key }, nil
	}
}
