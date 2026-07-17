package app

import (
	"sort"
	"strings"
)

// cluster is a set of dotted paths of equal depth that differ in at most one
// segment and carry one identical value. It prints as a single block whose
// varying segment lists every alternative found there, e.g.
// services.(api|worker).resources.limits.
type cluster struct {
	entries []*pathEntry
	segs    []string // segments of the first (representative) path
	values  []string // values seen at the varying segment, in path order
	varying int      // index of the single varying segment, -1 while none
}

// clusterPaths splits the paths sharing one value into printable clusters,
// each entry pre-sorted so a cluster's first entry is its smallest path. When
// merge is false every path stays its own cluster, keeping one block per path;
// when true (regex mode) paths of equal depth that differ in exactly one real
// segment are merged greedily into clusters. Paths that differ in depth or in
// more than one segment never merge, so an alternation only ever names
// combinations that were actually found.
func clusterPaths(entries []*pathEntry, merge bool) []*cluster {
	sorted := make([]*pathEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return natLess(sorted[i].path, sorted[j].path) })

	var clusters []*cluster
next:
	for _, e := range sorted {
		if merge {
			for _, c := range clusters {
				if c.tryAdd(e) {
					continue next
				}
			}
		}
		clusters = append(clusters, &cluster{entries: []*pathEntry{e}, segs: e.segs, varying: -1})
	}
	return clusters
}

// tryAdd extends the cluster with e when its path fits: the same depth, with
// every difference confined to the cluster's single varying segment, and both
// differing values readable inside an (a|b) alternation.
func (c *cluster) tryAdd(e *pathEntry) bool {
	if len(e.segs) != len(c.segs) {
		return false
	}
	diff := -1
	for i := range e.segs {
		if e.segs[i] == c.segs[i] {
			continue
		}
		if diff != -1 {
			return false
		}
		diff = i
	}
	if diff == -1 || (c.varying != -1 && diff != c.varying) {
		return false
	}
	if !safeAlt(c.segs[diff]) || !safeAlt(e.segs[diff]) {
		return false
	}
	if c.varying == -1 {
		c.varying = diff
		c.values = []string{c.segs[diff]}
	}
	c.values = append(c.values, e.segs[diff])
	c.entries = append(c.entries, e)
	return true
}

// safeAlt reports whether s stays unambiguous inside an (a|b) alternation:
// alternation metacharacters or an empty segment would garble the display, so
// such paths keep their own block instead.
func safeAlt(s string) bool {
	return s != "" && !strings.ContainsAny(s, "(|)")
}

// display renders the cluster's dotted path, replacing the varying segment
// with an (a|b) alternation listing every value found there.
func (c *cluster) display() string {
	if c.varying == -1 {
		return strings.Join(c.segs, ".")
	}
	segs := make([]string, len(c.segs))
	copy(segs, c.segs)
	segs[c.varying] = "(" + strings.Join(c.values, "|") + ")"
	return strings.Join(segs, ".")
}

// mergedFiles unions the files of every entry, deduplicated and in natural
// order. A single entry keeps its scan-order slice, which is already sorted.
func mergedFiles(entries []*pathEntry) []string {
	if len(entries) == 1 {
		return entries[0].files
	}
	seen := map[string]bool{}
	var files []string
	for _, e := range entries {
		for _, f := range e.files {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return natLess(files[i], files[j]) })
	return files
}
