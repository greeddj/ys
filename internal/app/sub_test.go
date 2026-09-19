package app

import (
	"strings"
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
		{"escaped delimiter is literal in RE even as a metacharacter", `s|a\|b|X|`, "a|b", "X"},
		{"an escaped metacharacter delimiter does not alternate", `s|a\|b|X|`, "b", "b"},
		{"an unescaped metacharacter still alternates", `s/a|b/X/`, "b", "X"},
		{"escaped delimiter is literal text in REPL", `s|x|a\|b|`, "x", "a|b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, err := parseSub(tt.expr)
			if err != nil {
				t.Fatalf("parseSub(%q): %v", tt.expr, err)
			}
			if got := applySubs(tt.in, nil, []subst{sub}); got != tt.want {
				t.Errorf("applySubs(%q, %q) = %q, want %q", tt.in, tt.expr, got, tt.want)
			}
		})
	}
}

// TestParseSubAddresses covers the sed-style address in front of a
// substitution: it selects by the value's whole dotted path, the same text -r
// matches, and ! inverts the selection.
func TestParseSubAddresses(t *testing.T) {
	tests := []struct {
		name string
		expr string
		path string
		in   string
		want string
	}{
		{"selects the key above the value", `/memory/s/\d+/N/`, "limits.memory", "2Gi", "NGi"},
		{"leaves a sibling key alone", `/memory/s/\d+/N/`, "limits.cpu", "2", "2"},
		{"reaches an ancestor", `/\.api\./s/\d+/N/`, "services.api.args.0", "8080", "N"},
		{"skips what the ancestor excludes", `/\.api\./s/\d+/N/`, "services.worker.args.0", "9090", "9090"},
		{"addresses a sequence index", `/args\.0$/s/\d+/N/`, "services.api.args.0", "8080", "N"},
		{"an index it does not name is skipped", `/args\.0$/s/\d+/N/`, "services.api.args.1", "3", "3"},
		{"matches the block's own path", `/^db\.timeout$/s/\d+/N/`, "db.timeout", "30s", "Ns"},
		{"negated address skips its match", `/memory/!s/\d+/N/`, "limits.memory", "2Gi", "2Gi"},
		{"negated address takes the rest", `/memory/!s/\d+/N/`, "limits.cpu", "2", "N"},
		{"custom address delimiter", `|memory|s/\d+/N/`, "limits.memory", "2Gi", "NGi"},
		{"escaped delimiter inside the address", `/a\/b/s/x/y/`, "a/b.k", "x", "y"},
		{"whitespace around address and bang", " /memory/ ! s/x/y/ ", "limits.cpu", "x", "y"},
		{"escaped metacharacter delimiter is literal", `|a\|b|s/x/y/`, "svc.a|b.k", "x", "y"},
		{"and does not alternate the address", `|memory\|cpu|s/x/y/`, "limits.cpu", "x", "x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, err := parseSub(tt.expr)
			if err != nil {
				t.Fatalf("parseSub(%q): %v", tt.expr, err)
			}
			got := applySubs(tt.in, strings.Split(tt.path, "."), []subst{sub})
			if got != tt.want {
				t.Errorf("applySubs(%q) at %q with %q = %q, want %q", tt.in, tt.path, tt.expr, got, tt.want)
			}
		})
	}
}

// TestParseSubUnaddressedIgnoresPath keeps the plain form path-blind: without
// an address a substitution reaches every value, wherever it sits.
func TestParseSubUnaddressedIgnoresPath(t *testing.T) {
	sub, err := parseSub(`s/\d+/N/`)
	if err != nil {
		t.Fatalf("parseSub: %v", err)
	}
	for _, path := range [][]string{nil, {"a"}, {"deeply", "nested", "0", "key"}} {
		if got := applySubs("2Gi", path, []subst{sub}); got != "NGi" {
			t.Errorf("applySubs at %q = %q, want %q", path, got, "NGi")
		}
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
		{"unterminated address", "/addr"},
		{"empty address", "//s/a/b/"},
		{"bad address regexp", "/((/s/a/b/"},
		{"address without a substitution", "/addr/"},
		{"address followed by another command", "/addr/x/a/b/"},
		{"backslash address delimiter", `\a\s/x/y/`},
		{"dangling backslash in the address", `/a\`},
		{"negation without a substitution", "/addr/!"},
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
	if got := applySubs("dev1", nil, subs); got != "PROD" {
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
	out, err := renderValue(normalize(doc.Content[0], nil, subs))
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
