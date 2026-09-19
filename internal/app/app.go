// Package app implements the ys core: it scans YAML files for a key and groups
// identical "path: value" results, printing each unique block once alongside the
// list of files it was found in. Useful for auditing config drift across many
// near-identical deployment value files.
package app

import (
	"context"
	"fmt"
	"io"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Options configures a single search run.
type Options struct {
	// Key is the search key, interpreted according to RegexMode/PathMode.
	Key string
	// Roots are the files and directories to scan.
	Roots []string
	// Subs are sed-style substitution expressions, s/RE/REPL/, applied in
	// order to the values inside each matched block before grouping, so blocks
	// differing only in, say, an environment name collapse into one. Keys and
	// paths are never rewritten.
	Subs []string
	// RegexMode matches Key as a regexp against the full dotted path. Paths of
	// equal depth that differ in one segment and carry an identical value are
	// merged into a single block, e.g. services.(api|worker).resources.limits.
	RegexMode bool
	// PathMode matches Key as a path suffix, e.g. http.timeout.
	PathMode bool
	// Color colorizes the output in the style of yq.
	Color bool
}

// hit is a single matched node.
type hit struct {
	node  *yaml.Node // normalized matched value, kept to render the final block
	path  string     // dotted path, e.g. services.api.http.timeout
	value string     // canonical rendering of the value alone, groups identical values
	segs  []string   // real path segments; a segment may itself contain dots
}

// pathEntry accumulates the files where one path carries one value. segs keeps
// the real segments so merging never confuses a key containing a literal dot
// with actual nesting.
type pathEntry struct {
	path  string
	segs  []string
	files []string
}

// valueGroup collects every path (and its files) carrying one identical value.
type valueGroup struct {
	node   *yaml.Node
	byPath map[string]*pathEntry
	paths  []*pathEntry
}

// group is one printed block with the files it was found in.
type group struct {
	sortPath string // natural-sort anchor: the smallest constituent path
	block    string // rendered "path: value" YAML block
	files    []string
}

// Run scans the YAML files under opts.Roots for opts.Key and writes the grouped
// results to stdout. Problems reading individual roots or files are reported to
// stderr and skipped; only an invalid key (bad regexp), an invalid substitution,
// or a cancelled context aborts the run.
func Run(ctx context.Context, stdout, stderr io.Writer, opts Options) error {
	match, err := buildMatcher(opts.Key, opts.RegexMode, opts.PathMode)
	if err != nil {
		return err
	}
	subs, err := parseSubs(opts.Subs)
	if err != nil {
		return err
	}

	files := collectFiles(stderr, opts.Roots)

	valueGroups := map[string]*valueGroup{}
	var order []*valueGroup
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		hits, err := scanFile(f, match, subs)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "# %s: %v\n", f, err)
			continue
		}
		for _, h := range hits {
			vg, ok := valueGroups[h.value]
			if !ok {
				vg = &valueGroup{node: h.node, byPath: map[string]*pathEntry{}}
				valueGroups[h.value] = vg
				order = append(order, vg)
			}
			pe, ok := vg.byPath[h.path]
			if !ok {
				pe = &pathEntry{path: h.path, segs: h.segs}
				vg.byPath[h.path] = pe
				vg.paths = append(vg.paths, pe)
			}
			if n := len(pe.files); n == 0 || pe.files[n-1] != f {
				pe.files = append(pe.files, f)
			}
		}
	}

	var groups []*group
	for _, vg := range order {
		for _, cl := range clusterPaths(vg.paths, opts.RegexMode) {
			display := cl.display()
			block, err := render(display, vg.node)
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "# %s: %v\n", display, err)
				continue
			}
			groups = append(groups, &group{
				sortPath: cl.entries[0].path,
				block:    block,
				files:    mergedFiles(cl.entries),
			})
		}
	}

	sortGroups(groups)
	printGroups(stdout, groups, opts.Color)
	return nil
}

// sortGroups orders groups so related paths are adjacent (a merged cluster is
// anchored at its smallest constituent path), the most common value comes
// first, and block text breaks ties for stable output.
func sortGroups(groups []*group) {
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.sortPath != b.sortPath {
			return natLess(a.sortPath, b.sortPath)
		}
		if len(a.files) != len(b.files) {
			return len(a.files) > len(b.files)
		}
		return a.block < b.block
	})
}

// printGroups writes each group as a header listing its files followed by the
// shared block, separating consecutive groups with a "---" document marker. When
// color is set, headers and separators are dimmed and the block is colorized in
// the style of yq.
func printGroups(w io.Writer, groups []*group, color bool) {
	for i, g := range groups {
		if i > 0 {
			_, _ = fmt.Fprintln(w, meta("---", color))
		}
		_, _ = fmt.Fprintln(w, meta(fmt.Sprintf("# %d file(s):", len(g.files)), color))
		for _, f := range g.files {
			_, _ = fmt.Fprintln(w, meta("#   "+f, color))
		}
		if color {
			_, _ = fmt.Fprintln(w, colorizeBlock(g.block))
		} else {
			_, _ = fmt.Fprintln(w, g.block)
		}
	}
}
