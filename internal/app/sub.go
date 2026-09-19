package app

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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

// parseSub parses one s<delim>RE<delim>REPL<delim> expression, ignoring any
// whitespace around it. The delimiter is the character right after the leading
// s: / by convention, but any character works, e.g. s|http://a/|http://b/|.
// Inside RE and REPL a backslash before the delimiter makes it literal, as sed
// has it: in RE the character reaches the regexp engine already escaped, so a
// delimiter that is a regexp metacharacter still means itself. Every other
// escape is left alone, so regexp escapes like \d pass through. Only RE is a
// regexp: REPL is inserted verbatim, with no $1 capture references, so a
// replacement may carry $ freely.
func parseSub(expr string) (subst, error) {
	fail := func(msg string) (subst, error) {
		return subst{}, fmt.Errorf("bad substitution %q: %s, want s/RE/REPL/", expr, msg)
	}
	e := trimSubSpace(expr)
	if len(e) < 2 || e[0] != 's' {
		return fail("no s<delimiter> prefix")
	}
	delim, size := utf8.DecodeRuneInString(e[1:])
	if delim == utf8.RuneError || delim == '\\' {
		return fail("bad delimiter")
	}
	pat, rest, closed, err := cutSub(e[1+size:], delim, regexp.QuoteMeta(string(delim)))
	if err != nil {
		return fail(err.Error())
	}
	if !closed {
		return fail("not exactly two delimited parts")
	}
	repl, tail, closed, err := cutSub(rest, delim, string(delim))
	if err != nil {
		return fail(err.Error())
	}
	if !closed || tail != "" {
		return fail("not exactly two delimited parts")
	}
	if pat == "" {
		return fail("empty pattern")
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return subst{}, fmt.Errorf("bad substitution %q: %w", expr, err)
	}
	return subst{re: re, repl: repl}, nil
}

// trimSubSpace drops the whitespace a shell can leave around an expression.
// Which side survived used to depend on how the flag was spelled, so neither
// is meaningful. The one exception is a whitespace delimiter, which is legal
// here: there the run at the end closes the last part instead of padding the
// expression, and the expression is left as written.
func trimSubSpace(expr string) string {
	e := strings.TrimLeftFunc(expr, unicode.IsSpace)
	if len(e) < 2 {
		return e
	}
	if delim, _ := utf8.DecodeRuneInString(e[1:]); unicode.IsSpace(delim) {
		return e
	}
	return strings.TrimRightFunc(e, unicode.IsSpace)
}

// cutSub reads the next delimiter-terminated part from the front of s. A
// backslash before the delimiter keeps it from closing the part and stands for
// literal instead, which the caller spells: the delimiter itself in text, its
// regexp escape in a pattern. Every other backslash escape is kept verbatim, so
// regexp escapes like \d pass through. rest is whatever followed the delimiter
// that closed the part. closed reports whether one did: when it did not, part
// holds the unterminated remainder and rest is empty.
func cutSub(s string, delim rune, literal string) (part, rest string, closed bool, err error) {
	var b strings.Builder
	esc := false
	for i, r := range s {
		switch {
		case esc:
			if r == delim {
				b.WriteString(literal)
			} else {
				b.WriteRune('\\')
				b.WriteRune(r)
			}
			esc = false
		case r == '\\':
			esc = true
		case r == delim:
			return b.String(), s[i+utf8.RuneLen(r):], true, nil
		default:
			b.WriteRune(r)
		}
	}
	if esc {
		return "", "", false, fmt.Errorf("dangling backslash")
	}
	return b.String(), "", false, nil
}

// applySubs runs every substitution over s in order, each one seeing the
// previous one's output. The replacement is inserted verbatim.
func applySubs(s string, subs []subst) string {
	for _, sub := range subs {
		s = sub.re.ReplaceAllLiteralString(s, sub.repl)
	}
	return s
}
