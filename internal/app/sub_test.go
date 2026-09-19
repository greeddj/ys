package app

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestParseSubApplies covers the accepted expression forms: each expression
// must parse and rewrite the sample input as sed would.
func TestParseSubApplies(t *testing.T) {
	tests := []struct {
		name string
		expr string
		in   string
		want string
	}{
		{"basic", `s/dev[0-9]+/NS/`, "dev12", "NS"},
		{"all occurrences", `s/a/b/`, "aaa", "bbb"},
		{"no match leaves input alone", `s/prod/NS/`, "dev1", "dev1"},
		{"custom delimiter", `s|http://old/|http://new/|`, "http://old/x", "http://new/x"},
		{"escaped delimiter", `s/a\/b/x/`, "a/b", "x"},
		{"regexp escapes pass through", `s/\d+/N/`, "a12", "aN"},
		{"replacement is literal, not expanded", `s/dev[0-9]+/$1-$HOME/`, "dev7", "$1-$HOME"},
		{"empty replacement deletes", `s/-suffix$//`, "name-suffix", "name"},
		{"template replacement", `s/dev\d+/{{ .Release.Namespace }}/`, "dev1", "{{ .Release.Namespace }}"},
		{"trailing whitespace ignored", "s/dev[0-9]+/NS/ ", "dev12", "NS"},
		{"leading whitespace ignored", " s/dev[0-9]+/NS/", "dev12", "NS"},
		{"whitespace on both sides ignored", "\t s/dev[0-9]+/NS/ \n", "dev12", "NS"},
		{"space delimiter survives trimming", "s dev[0-9]+ NS ", "dev12", "NS"},
		{"tab delimiter survives trimming", "s\tdev[0-9]+\tNS\t", "dev12", "NS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, err := parseSub(tt.expr)
			if err != nil {
				t.Fatalf("parseSub(%q): %v", tt.expr, err)
			}
			if got := applySubs(tt.in, []subst{sub}); got != tt.want {
				t.Errorf("applySubs(%q, %q) = %q, want %q", tt.in, tt.expr, got, tt.want)
			}
		})
	}
}

// TestParseSubErrors pins down what is rejected: anything short of exactly
// s<delim>RE<delim>REPL<delim> with a compilable RE.
func TestParseSubErrors(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"not a substitution", "y/a/b/"},
		{"bare s", "s"},
		{"backslash delimiter", `s\a\b\`},
		{"unterminated pattern", "s/a"},
		{"unterminated replacement", "s/a/b"},
		{"trailing text", "s/a/b/g"},
		{"extra part", "s/a/b/c/"},
		{"empty pattern", "s//b/"},
		{"bad regexp", "s/(/b/"},
		{"dangling backslash", `s/a/b\`},
		{"whitespace only", "   "},
		{"padding does not complete an expression", " s/a/b "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if sub, err := parseSub(tt.expr); err == nil {
				t.Errorf("parseSub(%q) = %+v, want an error", tt.expr, sub)
			}
		})
	}
}

// TestApplySubsInOrder checks that substitutions chain: each one sees the
// previous one's output.
func TestApplySubsInOrder(t *testing.T) {
	subs, err := parseSubs([]string{"s/dev/prod/", "s/prod1/PROD/"})
	if err != nil {
		t.Fatalf("parseSubs: %v", err)
	}
	if got := applySubs("dev1", subs); got != "PROD" {
		t.Errorf("applySubs chained = %q, want %q", got, "PROD")
	}
}

// substituted decodes a one-document YAML value and returns its canonical
// rendering after applying the substitution expressions, which is the text ys
// groups the value by.
func substituted(t *testing.T, src string, exprs ...string) string {
	t.Helper()
	subs, err := parseSubs(exprs)
	if err != nil {
		t.Fatalf("parseSubs(%q): %v", exprs, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", src, err)
	}
	out, err := renderValue(normalize(doc.Content[0], subs))
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return out
}

// TestSubstituteValues pins where a substitution reaches inside a block: every
// value at any depth, but never a key, and a changed scalar becomes a string.
func TestSubstituteValues(t *testing.T) {
	const sub = `s/dev\d+/NS/`
	tests := []struct {
		name string
		src  string
		expr string
		want string
	}{
		{"scalar block", "dev1", sub, "NS\n"},
		{"map value, not its key", "dev1: dev1", sub, "dev1: NS\n"},
		{"nested map value", "a:\n  dev1: dev1", sub, "a:\n  dev1: NS\n"},
		{"sequence elements", "[dev1, dev2]", sub, "- NS\n- NS\n"},
		{"structs in a sequence", "- name: dev1\n- name: dev2", sub, "- name: NS\n- name: NS\n"},
		{"complex key untouched", "? [dev1]\n: dev1", sub, "? - dev1\n: NS\n"},
		{"changed number becomes a string", "123", "s/1/9/", "\"923\"\n"},
		{"unchanged number keeps its type", "123", "s/7/9/", "123\n"},
		{"template needs quoting", "dev1", `s/dev\d+/{{ .Release.Namespace }}/`, "'{{ .Release.Namespace }}'\n"},
		{"sequence resorted after substitution", "[dev9, dev10]", "s/dev10/dev2/", "- dev2\n- dev9\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := substituted(t, tt.src, tt.expr); got != tt.want {
				t.Errorf("substituted(%q, %q) = %q, want %q", tt.src, tt.expr, got, tt.want)
			}
		})
	}
}

// TestSubstituteMergesAcrossTypes checks the retagging consequence: once a
// substitution rewrites both, the number 123 and the string "123" render alike
// and group together, the difference being erased on purpose.
func TestSubstituteMergesAcrossTypes(t *testing.T) {
	a := substituted(t, `foo: 123`, `s/\d+/N/`)
	b := substituted(t, `foo: "123"`, `s/\d+/N/`)
	if a != b {
		t.Errorf("substituted number and string should render alike:\n a = %q\n b = %q", a, b)
	}
}
