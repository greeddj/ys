package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// normalized decodes a one-document YAML value and returns its canonical
// rendering, which is the text ys groups identical values by.
func normalized(t *testing.T, src string) string {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal %q: %v", src, err)
	}
	out, err := renderValue(normalize(doc.Content[0], nil, nil))
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return out
}

// TestNormalizeEqualValues covers the notation that ys deliberately looks
// through: two values written differently but meaning the same must render
// identically, so they land in one group.
func TestNormalizeEqualValues(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
	}{
		{"double quotes", `foo: "text"`, `foo: text`},
		{"single quotes", `foo: 'text'`, `foo: text`},
		{"mixed quotes", `foo: "text"`, `foo: 'text'`},
		{"quoted key", `"foo": text`, `foo: text`},
		{"comment", `foo: text # incidental`, `foo: text`},
		{"mapping order", "b: 2\na: 1", "a: 1\nb: 2"},
		{"nested mapping order", "x:\n  b: 2\n  a: 1", "x:\n  a: 1\n  b: 2"},
		{"sequence order", "- c\n- a\n- b", "- a\n- b\n- c"},
		{"nested sequence order", "x:\n  - b\n  - a", "x:\n  - a\n  - b"},
		{"flow mapping", "{b: 2, a: 1}", "a: 1\nb: 2"},
		{"flow sequence", "[b, a]", "- a\n- b"},
		{"quotes inside a collection", `[a, "b"]`, `["a", b]`},
		{"quotes inside a mapping", `{k: "v"}`, `{k: v}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := normalized(t, tt.a), normalized(t, tt.b)
			if a != b {
				t.Errorf("%q and %q should normalize alike:\n a = %q\n b = %q", tt.a, tt.b, a, b)
			}
		})
	}
}

// TestNormalizeDistinctValues guards the other half: quoting that carries a type
// is not incidental, and collapsing it would hide exactly the drift ys exists to
// surface.
func TestNormalizeDistinctValues(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
	}{
		{"quoted number", `foo: "123"`, `foo: 123`},
		{"quoted bool", `foo: "true"`, `foo: true`},
		{"quoted null", `foo: "null"`, `foo: null`},
		{"empty string and null", `foo: ""`, `foo:`},
		{"different values", `foo: a`, `foo: b`},
		{"different keys", `a: 1`, `b: 1`},
		{"sequence and mapping", `[a]`, `{a: }`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := normalized(t, tt.a), normalized(t, tt.b)
			if a == b {
				t.Errorf("%q and %q must stay distinct, both normalized to %q", tt.a, tt.b, a)
			}
		})
	}
}

// TestNormalizeRendering pins the canonical text itself: sorted, unquoted, and
// still quoted wherever plain text would change the value.
func TestNormalizeRendering(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"drops quotes", `"text"`, "text\n"},
		{"keeps meaningful quotes", `"123"`, "\"123\"\n"},
		{"requotes what plain text would break", `"foo: bar"`, "'foo: bar'\n"},
		{"sorts mapping keys", "{b: 2, a: 1}", "a: 1\nb: 2\n"},
		{"sorts sequence entries", "[c, a, b]", "- a\n- b\n- c\n"},
		{"sorts keys naturally", "{env10: a, env2: b}", "env2: b\nenv10: a\n"},
		{"sorts entries naturally", "[env10, env2]", "- env2\n- env10\n"},
		{"sorts nested collections", "b:\n  - 2\n  - 1\na: {d: 4, c: 3}", "a:\n  c: 3\n  d: 4\nb:\n  - 1\n  - 2\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalized(t, tt.src); got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

// TestRenderKeepsALeadingLineBreak pins what the emitter does with a value that
// begins with a line break. A block header cannot carry that break on its own,
// so a blank line has to follow it; without one the block decodes back to a
// value the file does not hold, and the canonical text ys groups by would
// misstate it.
func TestRenderKeepsALeadingLineBreak(t *testing.T) {
	const src = "|2\n\n  hello\n"

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	val := doc.Content[0]
	if want := "\nhello\n"; val.Value != want {
		t.Fatalf("fixture decodes to %q, want %q", val.Value, want)
	}

	got := normalized(t, src)
	if want := "|2\n\n  hello\n"; got != want {
		t.Errorf("renderValue = %q, want %q", got, want)
	}

	var back string
	if err := yaml.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("decode rendering: %v", err)
	}
	if back != val.Value {
		t.Errorf("rendering decodes to %q, want %q", back, val.Value)
	}

	block, err := render("msg", normalize(val, nil, nil))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if want := "msg: |2\n\n  hello"; block != want {
		t.Errorf("render = %q, want %q", block, want)
	}
}

// TestNormalizeKeepsAliases checks that a block carrying an anchor or an alias
// still renders as before, rather than losing the anchor its alias refers to.
func TestNormalizeKeepsAliases(t *testing.T) {
	const src = "anchored: &d\n  cpu: 1\naliased: *d\n"
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m := doc.Content[0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		key := m.Content[i].Value
		before, err := renderValue(m.Content[i+1])
		if err != nil {
			t.Fatalf("render %s: %v", key, err)
		}
		after, err := renderValue(normalize(m.Content[i+1], nil, nil))
		if err != nil {
			t.Fatalf("render normalized %s: %v", key, err)
		}
		if before != after {
			t.Errorf("%s: normalize changed the rendering: %q -> %q", key, before, after)
		}
	}
}

// TestRunGroupsAcrossNotation is the end-to-end promise: files that only differ
// in quoting and in the order of a block's keys and entries collapse into one
// group, printed canonically.
func TestRunGroupsAcrossNotation(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.yaml", "limits:\n  cpu: \"500m\"\n  args: [b, a]\n")
	b := writeFile(t, dir, "b.yaml", "limits:\n  args:\n    - a\n    - b\n  cpu: 500m\n")
	c := writeFile(t, dir, "c.yaml", "limits: {args: ['a', 'b'], cpu: '500m'}\n")

	var out, errb bytes.Buffer
	if err := Run(context.Background(), &out, &errb, Options{Key: "limits", Roots: []string{dir}}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := "# 3 file(s):\n#   " + a + "\n#   " + b + "\n#   " + c +
		"\nlimits:\n  args:\n    - a\n    - b\n  cpu: 500m\n"
	if out.String() != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errb.String())
	}
}

// TestRunKeepsSequenceIndexesHonest checks that sorting a block's content never
// rewrites the paths: an index still names the element the file really holds.
func TestRunKeepsSequenceIndexesHonest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "args: [b, a]\n")

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: `^args\.0$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "args.0: b") {
		t.Errorf("args.0 should still be the file's first element:\n%s", out.String())
	}
}
