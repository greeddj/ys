package app

import (
	"sort"

	"gopkg.in/yaml.v3"
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
func normalize(n *yaml.Node) *yaml.Node {
	out := &yaml.Node{
		Kind:   n.Kind,
		Tag:    n.Tag,
		Value:  n.Value,
		Anchor: n.Anchor,
		Alias:  n.Alias,
	}
	if len(n.Content) == 0 {
		return out
	}
	out.Content = make([]*yaml.Node, len(n.Content))
	for i, c := range n.Content {
		out.Content[i] = normalize(c)
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
