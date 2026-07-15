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
)

// Options configures a single search run.
type Options struct {
	// Key is the search key, interpreted according to RegexMode/PathMode.
	Key string
	// Roots are the files and directories to scan.
	Roots []string
	// RegexMode matches Key as a regexp against the full dotted path.
	RegexMode bool
	// PathMode matches Key as a path suffix, e.g. settings.fozzy.
	PathMode bool
	// Color colorizes the output in the style of yq.
	Color bool
}

// hit is a single matched node rendered as a "path: value" block.
type hit struct {
	path  string // dotted path, e.g. secrets.crm.settings.fozzy
	block string // rendered "path: value" YAML block
}

// group collects the files that share one identical rendered block.
type group struct {
	path  string
	block string
	files []string
}

// Run scans the YAML files under opts.Roots for opts.Key and writes the grouped
// results to stdout. Problems reading individual roots or files are reported to
// stderr and skipped; only an invalid key (bad regexp) or a cancelled context
// aborts the run.
func Run(ctx context.Context, stdout, stderr io.Writer, opts Options) error {
	match, err := buildMatcher(opts.Key, opts.RegexMode, opts.PathMode)
	if err != nil {
		return err
	}

	files := collectFiles(stderr, opts.Roots)

	groupsByBlock := map[string]*group{}
	var groups []*group
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		hits, err := scanFile(f, match)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "# %s: %v\n", f, err)
			continue
		}
		for _, h := range hits {
			g, ok := groupsByBlock[h.block]
			if !ok {
				g = &group{path: h.path, block: h.block}
				groupsByBlock[h.block] = g
				groups = append(groups, g)
			}
			if n := len(g.files); n == 0 || g.files[n-1] != f {
				g.files = append(g.files, f)
			}
		}
	}

	sortGroups(groups)
	printGroups(stdout, groups, opts.Color)
	return nil
}

// sortGroups orders groups so identical paths are adjacent, the most common
// value within a path comes first, and value text breaks ties for stable output.
func sortGroups(groups []*group) {
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.path != b.path {
			return a.path < b.path
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
