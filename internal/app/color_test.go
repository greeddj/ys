package app

import (
	"strings"
	"testing"
)

func TestKeyColon(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"key: val", 3},
		{"key:", 3},
		{"a: b: c", 1},   // first "colon space" wins
		{"http://x", -1}, // ':' not followed by space or EOL
		{"item", -1},     // bare scalar, no key
		{`"a: b": v`, 6}, // ':' inside a double-quoted key is skipped
		{`'a''b': v`, 6}, // '' is an escaped quote inside a single-quoted key
		{"", -1},
	}
	for _, tt := range tests {
		if got := keyColon(tt.in); got != tt.want {
			t.Errorf("keyColon(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestIsBlockIndicator(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"|", true},
		{">", true},
		{"|-", true},
		{"|+", true},
		{">2", true},
		{"|2-", true},
		{"", false},
		{"x", false},
		{"|abc", false},
		{"| ", false},
	}
	for _, tt := range tests {
		if got := isBlockIndicator(tt.in); got != tt.want {
			t.Errorf("isBlockIndicator(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestColorScalar(t *testing.T) {
	tests := []struct {
		val  string
		want string
	}{
		{"hello", colStr},
		{`"true"`, colStr}, // quoted, so a string even though it reads like a bool
		{"'x'", colStr},
		{"null", colNull},
		{"~", colNull},
		{"true", colBool},
		{"False", colBool},
		{"3", colNum},
		{"-2.5", colNum},
		{".inf", colNum},
		{"{}", colMeta},
		{"[]", colMeta},
	}
	for _, tt := range tests {
		got := colorScalar(tt.val)
		if !strings.HasPrefix(got, tt.want) {
			t.Errorf("colorScalar(%q) = %q, want prefix %q", tt.val, got, tt.want)
		}
		if !strings.HasSuffix(got, colReset) {
			t.Errorf("colorScalar(%q) = %q, want reset suffix", tt.val, got)
		}
	}
}

func TestColorizeBlock(t *testing.T) {
	in := strings.Join([]string{
		"service:",
		"  n: 3",
		"  b: true",
		"  s: hello",
		"  x: null",
		`  q: "true"`,
		"  ports:",
		"    - 5432",
		"  cert: |",
		"    line: notkey",
	}, "\n")
	out := colorizeBlock(in)

	mustContain := []string{
		colKey + "service" + colReset,
		colKey + "n" + colReset,
		colNum + "3" + colReset,
		colBool + "true" + colReset,
		colStr + "hello" + colReset,
		colNull + "null" + colReset,
		colStr + `"true"` + colReset, // quoted value keeps string color
		colMeta + "- " + colReset,    // sequence marker
		colNum + "5432" + colReset,
		colMeta + "|" + colReset,           // block scalar indicator
		colStr + "line: notkey" + colReset, // block content is a string, not a key
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("colorized output missing %q\ngot:\n%s", want, out)
		}
	}

	// The block-scalar content line must not be parsed as a mapping key.
	if strings.Contains(out, colKey+"line"+colReset) {
		t.Errorf("block scalar content was mis-colored as a key\ngot:\n%s", out)
	}
}
