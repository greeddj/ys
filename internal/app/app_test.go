package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsYAML(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"a.yaml", true},
		{"a.yml", true},
		{"A.YAML", true},
		{"dir/b.YML", true},
		{"a.txt", false},
		{"a", false},
		{"a.yamlx", false},
		{"a.json", false},
	}
	for _, tt := range tests {
		if got := isYAML(tt.path); got != tt.want {
			t.Errorf("isYAML(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// writeFile creates dir/name with the given content, failing the test on error.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestRunGroupsOrdersAndIgnoresComments(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.yaml", "foo: enabled\n")
	b := writeFile(t, dir, "b.yaml", "foo: enabled  # incidental comment\n")
	c := writeFile(t, dir, "c.yaml", "foo: disabled\n")

	var out, errb bytes.Buffer
	if err := Run(context.Background(), &out, &errb, Options{Key: "foo", Roots: []string{dir}}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The most common value comes first; a and b group despite b's comment.
	want := fmt.Sprintf(
		"# 2 file(s):\n#   %s\n#   %s\nfoo: enabled\n---\n# 1 file(s):\n#   %s\nfoo: disabled\n",
		a, b, c,
	)
	if out.String() != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errb.String())
	}
}

func TestRunPathAndRegexModes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "v.yaml", "secrets:\n  crm:\n    settings:\n      dhcp: on-value\n  other:\n      dhcp: skip\n")

	t.Run("path suffix", func(t *testing.T) {
		var out bytes.Buffer
		if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
			Key: "settings.dhcp", Roots: []string{dir}, PathMode: true,
		}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !strings.Contains(out.String(), "secrets.crm.settings.dhcp: on-value") {
			t.Errorf("path mode missed the suffix match:\n%s", out.String())
		}
		if strings.Contains(out.String(), "secrets.other.dhcp") {
			t.Errorf("path mode matched the wrong key:\n%s", out.String())
		}
	})

	t.Run("regexp", func(t *testing.T) {
		var out bytes.Buffer
		if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
			Key: `^secrets\.other\.dhcp$`, Roots: []string{dir}, RegexMode: true,
		}); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !strings.Contains(out.String(), "secrets.other.dhcp: skip") {
			t.Errorf("regexp mode missed the match:\n%s", out.String())
		}
		if strings.Contains(out.String(), "settings.dhcp") {
			t.Errorf("regexp mode matched the wrong key:\n%s", out.String())
		}
	})
}

// TestRunSubstitutionCollapsesValues is the end-to-end promise of -s: blocks
// that differ only in an environment name inside the value collapse into one
// group once the substitution rewrites it, while untouched values keep their
// own group. Paths are never substituted; the collapsed group still merges its
// sibling paths into an alternation.
func TestRunSubstitutionCollapsesValues(t *testing.T) {
	dir := t.TempDir()
	d1 := writeFile(t, dir, "dev1.yaml", "envs:\n  dev1:\n    env_name: dev1\n")
	d2 := writeFile(t, dir, "dev2.yaml", "envs:\n  dev2:\n    env_name: dev2\n")
	sb := writeFile(t, dir, "sandbox.yaml", "envs:\n  sandbox1:\n    env_name: sandbox1\n")

	var out, errb bytes.Buffer
	if err := Run(context.Background(), &out, &errb, Options{
		Key:       `^envs\.[^.]+\.env_name$`,
		Roots:     []string{dir},
		RegexMode: true,
		Subs:      []string{`s/dev\d+/{{ .Release.Namespace }}/`},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := fmt.Sprintf(
		"# 2 file(s):\n#   %s\n#   %s\nenvs.(dev1|dev2).env_name: '{{ .Release.Namespace }}'\n"+
			"---\n# 1 file(s):\n#   %s\nenvs.sandbox1.env_name: sandbox1\n",
		d1, d2, sb,
	)
	if out.String() != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errb.String())
	}
}

// TestRunAddressedSubstitution is the end-to-end promise of an addressed -s:
// the rewrite reaches only the values whose dotted path the address selects, so
// two files collapse on the value that was normalized while the sibling key
// they already agreed on stays exactly as they hold it. With ! the selection
// flips and the blocks stay apart, since what still differs is untouched.
func TestRunAddressedSubstitution(t *testing.T) {
	const block = "services:\n  api:\n    resources:\n      limits:\n        cpu: \"1\"\n        memory: %s\n"

	tests := []struct {
		name string
		sub  string
		want string
	}{
		{
			name: "address narrows the rewrite to memory",
			sub:  `/memory/s/\d+/N/`,
			want: "# 2 file(s):\n#   %[1]s\n#   %[2]s\n" +
				"services.api.resources.limits:\n  cpu: \"1\"\n  memory: NGi\n",
		},
		{
			name: "negated address rewrites everything else",
			sub:  `/memory/!s/\d+/N/`,
			want: "# 1 file(s):\n#   %[1]s\nservices.api.resources.limits:\n  cpu: N\n  memory: 2Gi\n" +
				"---\n# 1 file(s):\n#   %[2]s\nservices.api.resources.limits:\n  cpu: N\n  memory: 4Gi\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			prod := writeFile(t, dir, "prod.yaml", fmt.Sprintf(block, "2Gi"))
			staging := writeFile(t, dir, "staging.yaml", fmt.Sprintf(block, "4Gi"))

			var out, errb bytes.Buffer
			if err := Run(context.Background(), &out, &errb, Options{
				Key:       `resources\.limits$`,
				Roots:     []string{dir},
				RegexMode: true,
				Subs:      []string{tt.sub},
			}); err != nil {
				t.Fatalf("Run: %v", err)
			}

			want := fmt.Sprintf(tt.want, prod, staging)
			if out.String() != want {
				t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
			}
			if errb.Len() != 0 {
				t.Errorf("unexpected stderr: %q", errb.String())
			}
		})
	}
}

func TestRunBadSubstitution(t *testing.T) {
	err := Run(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, Options{
		Key: "foo", Roots: []string{t.TempDir()}, Subs: []string{"nope"},
	})
	if err == nil {
		t.Fatal("expected an error for an invalid substitution, got nil")
	}
}

func TestRunBadRegexp(t *testing.T) {
	err := Run(context.Background(), &bytes.Buffer{}, &bytes.Buffer{}, Options{
		Key: "(", Roots: []string{t.TempDir()}, RegexMode: true,
	})
	if err == nil {
		t.Fatal("expected an error for an invalid regexp, got nil")
	}
}

func TestRunReportsMissingRootButContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "foo: enabled\n")
	missing := filepath.Join(dir, "does-not-exist")

	var out, errb bytes.Buffer
	if err := Run(context.Background(), &out, &errb, Options{Key: "foo", Roots: []string{missing, dir}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(errb.String(), "does-not-exist") {
		t.Errorf("missing root not reported to stderr: %q", errb.String())
	}
	if !strings.Contains(out.String(), "foo: enabled") {
		t.Errorf("existing root not scanned after a missing one:\n%s", out.String())
	}
}

func TestRunCanceledContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "foo: enabled\n")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Run(ctx, &bytes.Buffer{}, &bytes.Buffer{}, Options{Key: "foo", Roots: []string{dir}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run with canceled context = %v, want context.Canceled", err)
	}
}

func TestRunColorToggle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.yaml", "foo: enabled\n")

	const esc = "\x1b["

	var colored bytes.Buffer
	if err := Run(context.Background(), &colored, &bytes.Buffer{}, Options{Key: "foo", Roots: []string{dir}, Color: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(colored.String(), esc) {
		t.Errorf("Color:true produced no ANSI escapes:\n%q", colored.String())
	}

	var plain bytes.Buffer
	if err := Run(context.Background(), &plain, &bytes.Buffer{}, Options{Key: "foo", Roots: []string{dir}, Color: false}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(plain.String(), esc) {
		t.Errorf("Color:false leaked ANSI escapes:\n%q", plain.String())
	}
}
