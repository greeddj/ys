package app

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// subst is one compiled substitution: every match of re is replaced with repl,
// in the values whose dotted path addr selects. A nil addr selects every value;
// not inverts whatever addr selects.
type subst struct {
	re   *regexp.Regexp
	addr *regexp.Regexp
	repl string
	not  bool
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

// parseSub parses one substitution, with an optional sed-style address in
// front of it:
//
//	s<delim>RE<delim>REPL<delim>           rewrite every value in the block
//	/ADDR/s<delim>RE<delim>REPL<delim>     only where ADDR matches the path
//	/ADDR/!s<delim>RE<delim>REPL<delim>    only where it does not
//
// ADDR is a regexp over the whole dotted path of the value being rewritten,
// the same thing -r matches against, so it can name the key right above a
// value or any ancestor: /memory/ and /\.api\./ both work.
//
// Each part carries its own delimiter, the character that opens it, so /a/ and
// |a| are the same address; the substitution's delimiter is the character
// right after its s, which is / by convention but may be anything, as in
// s|http://a/|http://b/|. Inside a part, a backslash before that part's
// delimiter makes it literal, as sed does it: in a pattern the character is
// handed to the regexp engine already escaped, so a delimiter that is a regexp
// metacharacter still means itself. Every other escape is left alone, so
// regexp escapes like \d pass through. ADDR and RE are regexps; REPL is inserted
// verbatim, with no $1 capture references, so a replacement may carry $
// freely. Whitespace around the expression, and around the address and the !,
// is ignored.
func parseSub(expr string) (subst, error) {
	fail := func(msg string) (subst, error) {
		return subst{}, fmt.Errorf("bad substitution %q: %s, want s/RE/REPL/ or /ADDR/s/RE/REPL/", expr, msg)
	}
	e := strings.TrimLeftFunc(expr, unicode.IsSpace)

	var addr *regexp.Regexp
	var not bool
	// Anything but the s of the substitution itself opens an address, and
	// whatever opened it closes it.
	if e != "" && e[0] != 's' {
		delim, size := utf8.DecodeRuneInString(e)
		if delim == utf8.RuneError || delim == '\\' {
			return fail("bad address delimiter")
		}
		pat, rest, closed, err := cutSub(e[size:], delim, regexp.QuoteMeta(string(delim)))
		if err != nil {
			return fail(err.Error())
		}
		if !closed {
			return fail("unterminated address")
		}
		if pat == "" {
			return fail("empty address")
		}
		if addr, err = regexp.Compile(pat); err != nil {
			return subst{}, fmt.Errorf("bad substitution %q: bad address: %w", expr, err)
		}
		rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
		if strings.HasPrefix(rest, "!") {
			not = true
			rest = strings.TrimLeftFunc(rest[1:], unicode.IsSpace)
		}
		e = rest
	}

	e = trimSubTail(e)
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
	return subst{re: re, repl: repl, addr: addr, not: not}, nil
}

// trimSubTail drops the whitespace a shell can leave after an expression.
// Which side survived used to depend on how the flag was spelled, so neither
// is meaningful. The one exception is a whitespace delimiter, which is legal
// here: there the run at the end closes the last part instead of padding the
// expression, and the expression is left as written.
func trimSubTail(e string) string {
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
// previous one's output. An addressed substitution is skipped unless path, the
// dotted path of the value being rewritten, is selected by its address. The
// replacement is inserted verbatim.
func applySubs(s string, path []string, subs []subst) string {
	var dotted string
	joined := false
	for _, sub := range subs {
		if sub.addr != nil {
			if !joined {
				dotted, joined = strings.Join(path, "."), true
			}
			if sub.addr.MatchString(dotted) == sub.not {
				continue
			}
		}
		s = sub.re.ReplaceAllLiteralString(s, sub.repl)
	}
	return s
}
