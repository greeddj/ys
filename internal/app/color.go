package app

import (
	"regexp"
	"strings"
)

// ANSI SGR color codes chosen to resemble yq's colorized YAML output.
const (
	colReset = "\033[0m"
	colKey   = "\033[96m" // bright cyan: mapping keys
	colStr   = "\033[92m" // bright green: strings
	colNum   = "\033[95m" // bright magenta: numbers
	colBool  = "\033[93m" // bright yellow: booleans
	colNull  = "\033[90m" // bright black: null
	colMeta  = "\033[90m" // bright black: "# file" headers, "---", punctuation
)

// numberRe matches the plain scalars that YAML resolves to !!int or !!float,
// including the special float forms .inf/.nan.
var numberRe = regexp.MustCompile(
	`^[-+]?(0x[0-9a-fA-F]+|0o[0-7]+|(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?|\.(inf|Inf|INF|nan|NaN|NAN))$`,
)

// meta wraps a whole line (our own headers and separators) in the dim color when
// color is enabled.
func meta(s string, color bool) string {
	if !color {
		return s
	}
	return colMeta + s + colReset
}

// colorizeBlock colorizes a rendered "path: value" YAML block line by line.
func colorizeBlock(block string) string {
	lines := strings.Split(block, "\n")
	blockScalar := -1 // indent of the key owning an active block scalar, or -1
	for i, ln := range lines {
		lines[i] = colorizeLine(ln, &blockScalar)
	}
	return strings.Join(lines, "\n")
}

// colorizeLine colorizes a single YAML line. blockScalar tracks whether we are
// inside a literal/folded block scalar whose content lines must be treated as
// strings regardless of their shape.
func colorizeLine(line string, blockScalar *int) string {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	ws, rest := line[:indent], line[indent:]

	if *blockScalar >= 0 {
		if rest == "" || indent > *blockScalar {
			if rest == "" {
				return line
			}
			return ws + colStr + rest + colReset
		}
		*blockScalar = -1 // dedented out of the block scalar; parse normally
	}

	var b strings.Builder
	b.WriteString(ws)

	// Sequence markers can nest (e.g. "- - value").
	for rest == "-" || strings.HasPrefix(rest, "- ") {
		if rest == "-" {
			b.WriteString(colMeta + "-" + colReset)
			return b.String()
		}
		b.WriteString(colMeta + "- " + colReset)
		rest = rest[2:]
		indent += 2
	}

	if ci := keyColon(rest); ci >= 0 {
		b.WriteString(colKey + rest[:ci] + colReset)
		b.WriteByte(':')
		after := rest[ci+1:]
		if after == "" {
			return b.String() // "key:" with a nested block on the following lines
		}
		gap := 0
		for gap < len(after) && after[gap] == ' ' {
			gap++
		}
		b.WriteString(after[:gap])
		writeValue(&b, after[gap:], indent, blockScalar)
		return b.String()
	}

	writeValue(&b, rest, indent, blockScalar)
	return b.String()
}

// writeValue colorizes a scalar value, entering block-scalar mode when it is a
// literal/folded indicator ("|", ">", with optional chomping/indent modifiers).
func writeValue(b *strings.Builder, val string, indent int, blockScalar *int) {
	if val == "" {
		return
	}
	if isBlockIndicator(val) {
		b.WriteString(colMeta + val + colReset)
		*blockScalar = indent
		return
	}
	b.WriteString(colorScalar(val))
}

// colorScalar returns val wrapped in the color for its resolved YAML type.
func colorScalar(val string) string {
	switch {
	case strings.HasPrefix(val, `"`) || strings.HasPrefix(val, `'`):
		return colStr + val + colReset
	case val == "null" || val == "Null" || val == "NULL" || val == "~":
		return colNull + val + colReset
	case isBool(val):
		return colBool + val + colReset
	case numberRe.MatchString(val):
		return colNum + val + colReset
	case val == "{}" || val == "[]":
		return colMeta + val + colReset
	default:
		return colStr + val + colReset
	}
}

func isBool(val string) bool {
	switch val {
	case "true", "True", "TRUE", "false", "False", "FALSE":
		return true
	default:
		return false
	}
}

// isBlockIndicator reports whether val is a "|" or ">" block scalar header,
// possibly carrying chomping ("+"/"-") and explicit-indent digits.
func isBlockIndicator(val string) bool {
	if len(val) == 0 || (val[0] != '|' && val[0] != '>') {
		return false
	}
	for i := 1; i < len(val); i++ {
		switch val[i] {
		case '+', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		default:
			return false
		}
	}
	return true
}

// keyColon returns the index of the ':' separating a mapping key from its value
// in s, or -1 when s is not a mapping entry. It respects single- and
// double-quoted keys so a ':' inside a quoted key is not mistaken for the
// separator.
func keyColon(s string) int {
	if s == "" {
		return -1
	}
	switch s[0] {
	case '"':
		for i := 1; i < len(s); i++ {
			if s[i] == '\\' {
				i++
				continue
			}
			if s[i] == '"' {
				return colonAt(s, i+1)
			}
		}
		return -1
	case '\'':
		for i := 1; i < len(s); i++ {
			if s[i] == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					i++ // '' is an escaped quote inside a single-quoted scalar
					continue
				}
				return colonAt(s, i+1)
			}
		}
		return -1
	default:
		for i := 0; i < len(s); i++ {
			if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ') {
				return i
			}
		}
		return -1
	}
}

// colonAt returns idx when s[idx] is a key/value separating colon, else -1.
func colonAt(s string, idx int) int {
	if idx < len(s) && s[idx] == ':' && (idx+1 == len(s) || s[idx+1] == ' ') {
		return idx
	}
	return -1
}
