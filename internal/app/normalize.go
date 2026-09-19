package app

import (
	"sort"
	"strconv"

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
//
// path is where the block itself sits, so that an addressed substitution sees
// the same dotted path the scan matched against, extended by the key or index
// of every value below it.
func normalize(n *yaml.Node, path []string, subs []subst) *yaml.Node {
	z := normalizer{subs: subs}
	for _, s := range subs {
		if s.addr != nil {
			z.addressed = true
			break
		}
	}
	if !z.addressed {
		path = nil
	}
	return z.node(n, path, true)
}

// normalizer carries down what the copy needs besides the node itself.
type normalizer struct {
	subs      []subst
	addressed bool
}

// node copies n applying the normalize rules, substituting only while value
// stays true: it starts true at the matched block and is dropped for a mapping
// key and everything inside it.
func (z normalizer) node(n *yaml.Node, path []string, value bool) *yaml.Node {
	out := &yaml.Node{
		Kind:   n.Kind,
		Tag:    n.Tag,
		Value:  n.Value,
		Anchor: n.Anchor,
		Alias:  n.Alias,
	}
	if value && n.Kind == yaml.ScalarNode {
		if v := applySubs(n.Value, path, z.subs); v != n.Value {
			out.Value = v
			out.Tag = "!!str"
		}
	}
	if len(n.Content) == 0 {
		return out
	}
	out.Content = make([]*yaml.Node, len(n.Content))
	for i, c := range n.Content {
		out.Content[i] = z.node(c, z.childPath(n, path, i), value && (n.Kind != yaml.MappingNode || i%2 == 1))
	}
	switch n.Kind {
	case yaml.MappingNode:
		sortEntries(out.Content, 2)
	case yaml.SequenceNode:
		sortEntries(out.Content, 1)
	}
	return out
}

// childPath names where content[i] sits, the way the scan names it: a mapping's
// value takes its key, a sequence's element takes its index. Sorting comes
// after this, so an index still counts positions in the file. Only an addressed
// substitution ever reads a path, so an unaddressed copy builds none.
func (z normalizer) childPath(n *yaml.Node, path []string, i int) []string {
	if !z.addressed {
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		if i%2 == 0 {
			return path // a key is never substituted, so its own path goes unused
		}
		return childPath(path, n.Content[i-1].Value)
	case yaml.SequenceNode:
		return childPath(path, strconv.Itoa(i))
	default:
		return path
	}
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
