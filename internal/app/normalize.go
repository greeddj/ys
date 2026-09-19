package app

import (
	"sort"

	"go.yaml.in/yaml/v3"
)

// normalize returns a canonical deep copy of n, so that values differing only in
// incidental notation render to the same text and group together: comments are
// dropped, quoting and flow styles are dropped, and the entries of every nested
// mapping and sequence are sorted.
//
// Tags are preserved, so the emitter restores whatever quoting a value needs to
// keep its type: "123" stays a quoted string and never merges with the number
// 123, and "" stays an empty string instead of collapsing to null. Anchors and
// aliases are copied verbatim, keeping a block's rendering unchanged.
//
// Each substitution in subs rewrites the values of the copy - the matched block
// itself when it is a scalar, a mapping's values, a sequence's elements - and is
// applied before the surrounding collection is sorted, so grouping and ordering
// both see the substituted text. Keys are never rewritten. A scalar changed by
// a substitution becomes a string, whatever its original type was.
func normalize(n *yaml.Node, subs []subst) *yaml.Node {
	return normalizeIn(n, subs, true)
}

// normalizeIn copies n applying the normalize rules, substituting only while
// value stays true: it starts true at the matched block and is dropped for a
// mapping key and everything inside it.
func normalizeIn(n *yaml.Node, subs []subst, value bool) *yaml.Node {
	out := &yaml.Node{
		Kind:   n.Kind,
		Tag:    n.Tag,
		Value:  n.Value,
		Anchor: n.Anchor,
		Alias:  n.Alias,
	}
	if value && n.Kind == yaml.ScalarNode {
		if v := applySubs(n.Value, subs); v != n.Value {
			out.Value = v
			out.Tag = "!!str"
		}
	}
	if len(n.Content) == 0 {
		return out
	}
	out.Content = make([]*yaml.Node, len(n.Content))
	for i, c := range n.Content {
		out.Content[i] = normalizeIn(c, subs, value && (n.Kind != yaml.MappingNode || i%2 == 1))
	}
	switch n.Kind {
	case yaml.MappingNode:
		sortEntries(out.Content, 2)
	case yaml.SequenceNode:
		sortEntries(out.Content, 1)
	}
	return out
}

// entry is one collection element together with the text ordering it.
type entry struct {
	order string       // canonical text of the node deciding the order
	nodes []*yaml.Node // the element itself, or a mapping's key and value
}

// sortEntries reorders content in place, reading it as elements of stride nodes
// each (1 in a sequence, a key and its value in a mapping) and ordering them
// naturally by the canonical text of the first node of each: the element itself
// in a sequence, the key in a mapping. A trailing partial element, which a
// well-formed mapping never has, keeps its position.
func sortEntries(content []*yaml.Node, stride int) {
	entries := make([]entry, len(content)/stride)
	for i := range entries {
		nodes := make([]*yaml.Node, stride)
		copy(nodes, content[i*stride:])
		entries[i] = entry{order: canonical(nodes[0]), nodes: nodes}
	}
	sort.SliceStable(entries, func(i, j int) bool { return natLess(entries[i].order, entries[j].order) })
	for i, e := range entries {
		copy(content[i*stride:], e.nodes)
	}
}

// canonical returns the text a node is ordered by: its own rendering, so that
// elements differing only in type (the string "123" and the number 123) still
// order deterministically. A node that cannot be rendered sorts first, which
// only ever matters against another such node.
func canonical(n *yaml.Node) string {
	s, err := renderValue(n)
	if err != nil {
		return ""
	}
	return s
}
