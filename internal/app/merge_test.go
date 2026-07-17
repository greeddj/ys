package app

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestClusterPaths(t *testing.T) {
	entries := func(paths ...string) []*pathEntry {
		out := make([]*pathEntry, len(paths))
		for i, p := range paths {
			out[i] = &pathEntry{path: p, segs: strings.Split(p, ".")}
		}
		return out
	}
	displays := func(clusters []*cluster) []string {
		out := make([]string, len(clusters))
		for i, c := range clusters {
			out[i] = c.display()
		}
		return out
	}

	tests := []struct {
		name  string
		paths []string
		want  []string
		merge bool
	}{
		{
			name:  "one differing segment merges",
			paths: []string{"secrets.crm.settings.auth", "secrets.compute.settings.auth"},
			merge: true,
			want:  []string{"secrets.(compute|crm).settings.auth"},
		},
		{
			name:  "three-way alternation",
			paths: []string{"a.z.k", "a.m.k", "a.b.k"},
			merge: true,
			want:  []string{"a.(b|m|z).k"},
		},
		{
			name:  "different depth stays separate",
			paths: []string{"secrets.crm.auth", "secrets.crm.settings.auth"},
			merge: true,
			want:  []string{"secrets.crm.auth", "secrets.crm.settings.auth"},
		},
		{
			name:  "two differing segments stay separate",
			paths: []string{"a.x.c", "b.y.c"},
			merge: true,
			want:  []string{"a.x.c", "b.y.c"},
		},
		{
			name:  "cross product never invents combinations",
			paths: []string{"a.x.c", "a.x.d", "a.y.c", "a.y.d"},
			merge: true,
			want:  []string{"a.x.(c|d)", "a.y.(c|d)"},
		},
		{
			name:  "numeric segments keep natural order",
			paths: []string{"envs.staging10.auth", "envs.staging2.auth", "envs.staging1.auth"},
			merge: true,
			want:  []string{"envs.(staging1|staging2|staging10).auth"},
		},
		{
			name:  "segment with alternation metacharacters stays separate",
			paths: []string{"s.a|b.k", "s.c.k"},
			merge: true,
			want:  []string{"s.a|b.k", "s.c.k"},
		},
		{
			name:  "merge disabled keeps one cluster per path",
			paths: []string{"secrets.crm.auth", "secrets.compute.auth"},
			merge: false,
			want:  []string{"secrets.compute.auth", "secrets.crm.auth"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := displays(clusterPaths(entries(tt.paths...), tt.merge))
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("clusterPaths(%v, merge=%v):\ngot  %v\nwant %v", tt.paths, tt.merge, got, tt.want)
			}
		})
	}
}

func TestClusterPathsRealSegments(t *testing.T) {
	// A key literally named "a.b" flattens to the same dotted string as real
	// nesting a -> b; clustering must trust the real segments, not the dots.
	dottedKey := &pathEntry{path: "secrets.a.b.auth", segs: []string{"secrets", "a.b", "auth"}}
	nested := &pathEntry{path: "secrets.a.c.auth", segs: []string{"secrets", "a", "c", "auth"}}
	clusters := clusterPaths([]*pathEntry{dottedKey, nested}, true)
	if len(clusters) != 2 {
		t.Fatalf("dotted key merged with real nesting: %d cluster(s)", len(clusters))
	}

	// Two dotted keys of the same real depth merge; the alternation lists the
	// literal segment values.
	other := &pathEntry{path: "secrets.a.c.auth", segs: []string{"secrets", "a.c", "auth"}}
	clusters = clusterPaths([]*pathEntry{dottedKey, other}, true)
	if len(clusters) != 1 || clusters[0].display() != "secrets.(a.b|a.c).auth" {
		t.Fatalf("dotted keys of equal depth must merge, got %d cluster(s), first %q",
			len(clusters), clusters[0].display())
	}
}

func TestRunRegexpDottedKeyNotMergedWithNesting(t *testing.T) {
	dir := t.TempDir()
	// f1 has a single key literally named "a.b"; f2 nests a -> c. Their dotted
	// paths differ in depth for real, so the identical value must not merge.
	writeFile(t, dir, "f1.yaml", "secrets:\n  \"a.b\":\n    auth: v\n")
	writeFile(t, dir, "f2.yaml", "secrets:\n  a:\n    c:\n      auth: v\n")

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: `auth$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "|") {
		t.Errorf("dotted key must not merge with real nesting:\n%s", got)
	}
	for _, block := range []string{"secrets.a.b.auth: v", "secrets.a.c.auth: v"} {
		if !strings.Contains(got, block) {
			t.Errorf("missing separate block %q:\n%s", block, got)
		}
	}
}

func TestRenderKeepsOneLineShapeForLongKeys(t *testing.T) {
	val := &yaml.Node{Kind: yaml.ScalarNode, Value: "v"}

	// Around the yaml.v3 simple-key limit the shape must not change.
	for _, n := range []int{yamlMaxSimpleKey, yamlMaxSimpleKey + 1, 3 * yamlMaxSimpleKey} {
		key := strings.Repeat("k", n)
		block, err := render(key, val)
		if err != nil {
			t.Fatalf("render(len %d): %v", n, err)
		}
		if want := key + ": v"; block != want {
			t.Errorf("render(len %d) lost the one-line shape:\n%s", n, block)
		}
	}

	// A long key that is not plain-safe gets single-quoted, with quotes escaped.
	key := strings.Repeat("k", yamlMaxSimpleKey) + ": 'x"
	block, err := render(key, val)
	if err != nil {
		t.Fatalf("render(quoted): %v", err)
	}
	if want := "'" + strings.Repeat("k", yamlMaxSimpleKey) + ": ''x': v"; block != want {
		t.Errorf("long unsafe key not single-quoted:\ngot  %s\nwant %s", block, want)
	}
}

func TestRunRegexpLongAlternationStaysOneLine(t *testing.T) {
	dir := t.TempDir()
	var envs []string
	var b strings.Builder
	b.WriteString("secrets:\n")
	for _, env := range []string{"one", "two", "three", "four", "five", "six"} {
		name := "alpha-environment-" + env
		envs = append(envs, name)
		fmt.Fprintf(&b, "  %s:\n    auth: shared\n", name)
	}
	writeFile(t, dir, "a.yaml", b.String())

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: `auth$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	sort.Strings(envs) // the alternation lists values in natural path order
	want := "secrets.(" + strings.Join(envs, "|") + ").auth: shared\n"
	if len(want) <= yamlMaxSimpleKey {
		t.Fatalf("fixture too short to cross the simple-key limit: %d", len(want))
	}
	if !strings.Contains(got, want) {
		t.Errorf("long merged key lost the one-line \"path: value\" shape:\n%s", got)
	}
	if strings.Contains(got, "? ") {
		t.Errorf("explicit-key form leaked into the output:\n%s", got)
	}
}

func TestRunRegexpMergesAcrossDocumentsInOneFile(t *testing.T) {
	dir := t.TempDir()
	f := writeFile(t, dir, "multi.yaml",
		"secrets:\n  crm:\n    auth: same\n---\nsecrets:\n  compute:\n    auth: same\n")

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: `auth$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := fmt.Sprintf("# 1 file(s):\n#   %s\nsecrets.(compute|crm).auth: same\n", f)
	if out.String() != want {
		t.Errorf("multi-document merge mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
}

func TestRunRegexpMergesIdenticalValues(t *testing.T) {
	dir := t.TempDir()
	shared := "    settings:\n      auth:\n        api_secret: 'Secret1'\n        crm_secret: Secret2\n"
	crm := writeFile(t, dir, "crm.yaml", "secrets:\n  crm:\n"+shared)
	compute := writeFile(t, dir, "compute.yaml", "secrets:\n  compute:\n"+shared)
	portal := writeFile(t, dir, "portal.yaml",
		"secrets:\n  portal:\n"+shared+"        portal_secret: Secret3\n")

	var out, errb bytes.Buffer
	if err := Run(context.Background(), &out, &errb, Options{
		Key: `^secrets\..*\.settings\.auth$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// crm and compute share the value and merge into one alternation block;
	// portal's extra key keeps it a separate group.
	want := fmt.Sprintf(
		"# 2 file(s):\n#   %s\n#   %s\n"+
			"secrets.(compute|crm).settings.auth:\n  api_secret: 'Secret1'\n  crm_secret: Secret2\n"+
			"---\n"+
			"# 1 file(s):\n#   %s\n"+
			"secrets.portal.settings.auth:\n  api_secret: 'Secret1'\n  crm_secret: Secret2\n  portal_secret: Secret3\n",
		compute, crm, portal,
	)
	if out.String() != want {
		t.Errorf("output mismatch:\n--- got ---\n%s\n--- want ---\n%s", out.String(), want)
	}
	if errb.Len() != 0 {
		t.Errorf("unexpected stderr: %q", errb.String())
	}
}

func TestRunExactModeDoesNotMerge(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "crm.yaml", "secrets:\n  crm:\n    auth: same\n")
	writeFile(t, dir, "compute.yaml", "secrets:\n  compute:\n    auth: same\n")

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: "auth", Roots: []string{dir},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "|") {
		t.Errorf("exact mode must not merge paths:\n%s", got)
	}
	for _, path := range []string{"secrets.compute.auth: same", "secrets.crm.auth: same"} {
		if !strings.Contains(got, path) {
			t.Errorf("missing separate block %q:\n%s", path, got)
		}
	}
}

func TestRunRegexpMergedGroupSortsWithItsFamily(t *testing.T) {
	dir := t.TempDir()
	// compute and crm share one value; a second crm-only key under the same
	// parent must sort right after the merged block, not away from it.
	f := writeFile(t, dir, "a.yaml", "secrets:\n  compute:\n    auth: shared\n  crm:\n    auth: shared\n  zeta:\n    auth: other\n")

	var out bytes.Buffer
	if err := Run(context.Background(), &out, &bytes.Buffer{}, Options{
		Key: `auth$`, Roots: []string{dir}, RegexMode: true,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := out.String()
	merged := strings.Index(got, "secrets.(compute|crm).auth: shared")
	zeta := strings.Index(got, "secrets.zeta.auth: other")
	if merged == -1 || zeta == -1 {
		t.Fatalf("expected blocks missing:\n%s", got)
	}
	if merged > zeta {
		t.Errorf("merged group must sort by its smallest path (compute < zeta):\n%s", got)
	}
	// Both merged paths live in the same file: it must be counted and listed
	// once per group, not once per constituent path.
	if strings.Contains(got, "# 2 file(s):") {
		t.Errorf("one file with two merged paths counted twice:\n%s", got)
	}
	if n := strings.Count(got, "#   "+f+"\n"); n != 2 {
		t.Errorf("file must be listed once in each of the two groups, listed %d time(s):\n%s", n, got)
	}
}
