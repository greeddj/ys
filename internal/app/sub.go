package app

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// subst is one compiled substitution: every match of re is replaced with repl.
type subst struct {
	re   *regexp.Regexp
	repl string
}

// parseSubs compiles the sed-style substitution expressions given on the
// command line, e.g. s/dev\d+/{{ .Release.Namespace }}/, in the order they
// will be applied.
func parseSubs(exprs []string) ([]subst, error) {
	if len(exprs) == 0 {
		return nil, nil
	}
	subs := make([]subst, len(exprs))
	for i, e := range exprs {
		s, err := parseSub(e)
		if err != nil {
			return nil, err
		}
		subs[i] = s
	}
	return subs, nil
}

// parseSub parses one s<delim>RE<delim>REPL<delim> expression. The delimiter
// is the character right after the leading s: / by convention, but any
// character works, e.g. s|http://a/|http://b/|. Inside RE and REPL a
// backslash before the delimiter makes it literal; every other escape is left
// alone, so regexp escapes like \d pass through. Only RE is a regexp: REPL is
// inserted verbatim, with no $1 capture references, so a replacement may carry
// $ freely.
func parseSub(expr string) (subst, error) {
	fail := func(msg string) (subst, error) {
		return subst{}, fmt.Errorf("bad substitution %q: %s, want s/RE/REPL/", expr, msg)
	}
	if len(expr) < 2 || expr[0] != 's' {
		return fail("no s<delimiter> prefix")
	}
	delim, size := utf8.DecodeRuneInString(expr[1:])
	if delim == utf8.RuneError || delim == '\\' {
		return fail("bad delimiter")
	}
	parts, tail, err := splitSub(expr[1+size:], delim)
	if err != nil {
		return fail(err.Error())
	}
	if len(parts) != 2 || tail != "" {
		return fail("not exactly two delimited parts")
	}
	if parts[0] == "" {
		return fail("empty pattern")
	}
	re, err := regexp.Compile(parts[0])
	if err != nil {
		return subst{}, fmt.Errorf("bad substitution %q: %w", expr, err)
	}
	return subst{re: re, repl: parts[1]}, nil
}

// splitSub cuts s into its delimiter-terminated parts, unescaping \<delim> and
// keeping every other backslash escape verbatim. tail is whatever followed the
// last closed part; a complete substitution leaves it empty.
func splitSub(s string, delim rune) (parts []string, tail string, err error) {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case esc:
			if r != delim {
				b.WriteRune('\\')
			}
			b.WriteRune(r)
			esc = false
		case r == '\\':
			esc = true
		case r == delim:
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if esc {
		return nil, "", fmt.Errorf("dangling backslash")
	}
	return parts, b.String(), nil
}

// applySubs runs every substitution over s in order, each one seeing the
// previous one's output. The replacement is inserted verbatim.
func applySubs(s string, subs []subst) string {
	for _, sub := range subs {
		s = sub.re.ReplaceAllLiteralString(s, sub.repl)
	}
	return s
}
