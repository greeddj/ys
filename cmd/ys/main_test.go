package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestColorFlagWiring guards the -c/-n flag names against the wiring drifting
// from the registered flag names: an unknown name silently reads as false and
// turns the flag into a no-op.
func TestColorFlagWiring(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("foo: bar\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := newCommand()
		cmd.Writer = &out
		cmd.ErrWriter = &bytes.Buffer{}
		if err := cmd.Run(context.Background(), append([]string{"ys"}, args...)); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return out.String()
	}

	if got := run("-c", "foo", dir); !strings.Contains(got, "\x1b[") {
		t.Errorf("-c must force ANSI color, got %q", got)
	}
	if got := run("-n", "foo", dir); strings.Contains(got, "\x1b[") {
		t.Errorf("-n must disable ANSI color, got %q", got)
	}
}

// TestSubFlagWiring guards the -s flag the same way: an unknown name reads as
// an empty slice and silently drops the substitutions.
func TestSubFlagWiring(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"a.yaml": "foo: dev1\n", "b.yaml": "foo: dev2\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	var out bytes.Buffer
	cmd := newCommand()
	cmd.Writer = &out
	cmd.ErrWriter = &bytes.Buffer{}
	if err := cmd.Run(context.Background(), []string{"ys", "-n", "-s", `s/dev\d+/NS/`, "foo", dir}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "# 2 file(s):") || !strings.Contains(got, "foo: NS") {
		t.Errorf("-s must substitute values before grouping, got:\n%s", got)
	}
}

// TestSubFlagKeepsCommas guards DisableSliceFlagSeparator: by default a slice
// flag splits its value on commas, tearing apart any substitution containing
// one, whether a literal comma or a {1,2} quantifier.
func TestSubFlagKeepsCommas(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("foo: \"['POOL', 5]\"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	var out bytes.Buffer
	cmd := newCommand()
	cmd.Writer = &out
	cmd.ErrWriter = &bytes.Buffer{}
	args := []string{"ys", "-n", "-s", `s/POOL', \d{1,2}/POOL', 32/`, "foo", dir}
	if err := cmd.Run(context.Background(), args); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, `foo: '[''POOL'', 32]'`) {
		t.Errorf("a substitution with commas must stay whole, got:\n%s", got)
	}
}
