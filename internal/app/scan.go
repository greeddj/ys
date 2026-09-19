package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// collectFiles gathers the unique YAML files reachable from roots, walking
// directories recursively. Unreadable roots are reported to errw and skipped;
// the returned list is sorted in natural order so "staging2" precedes
// "staging10" rather than following it.
func collectFiles(errw io.Writer, roots []string) []string {
	var files []string
	seen := map[string]bool{}
	add := func(p string) {
		if isYAML(p) && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for _, r := range roots {
		info, err := os.Stat(r)
		if err != nil {
			_, _ = fmt.Fprintf(errw, "# %s: %v\n", r, err)
			continue
		}
		if info.IsDir() {
			_ = filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil //nolint:nilerr // skip unreadable entries and keep walking
				}
				if !d.IsDir() {
					add(p)
				}
				return nil
			})
		} else {
			add(r)
		}
	}
	sort.Slice(files, func(i, j int) bool { return natLess(files[i], files[j]) })
	return files
}

// isYAML reports whether p has a .yml or .yaml extension.
func isYAML(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".yml", ".yaml":
		return true
	default:
		return false
	}
}

// scanFile decodes every YAML document in file and returns the blocks whose
// dotted path matches, with subs applied to each block's values.
func scanFile(file string, match matcher, subs []subst) ([]hit, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var hits []hit
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		walk(&doc, nil, match, subs, &hits)
	}
	return hits, nil
}

// walk visits every node, recording a hit whenever its dotted path matches.
func walk(n *yaml.Node, path []string, match matcher, subs []subst, out *[]hit) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			walk(c, path, match, subs, out)
		}
	case yaml.MappingNode:
		check(n, path, match, subs, out)
		for i := 0; i+1 < len(n.Content); i += 2 {
			walk(n.Content[i+1], childPath(path, n.Content[i].Value), match, subs, out)
		}
	case yaml.SequenceNode:
		check(n, path, match, subs, out)
		for i, c := range n.Content {
			walk(c, childPath(path, strconv.Itoa(i)), match, subs, out)
		}
	case yaml.ScalarNode:
		check(n, path, match, subs, out)
	}
}

// childPath returns a fresh slice so sibling recursion never shares backing storage.
func childPath(path []string, seg string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = seg
	return next
}

// check appends a hit when the node at path matches and renders cleanly. Only
// the matched value is normalized and substituted, never the document, so the
// indexes in a path keep pointing at the element the file really holds there.
func check(n *yaml.Node, path []string, match matcher, subs []subst, out *[]hit) {
	if len(path) == 0 {
		return
	}
	dotted := strings.Join(path, ".")
	if !match(dotted, path[len(path)-1]) {
		return
	}
	node := normalize(n, subs)
	value, err := renderValue(node)
	if err != nil {
		return
	}
	*out = append(*out, hit{path: dotted, segs: path, value: value, node: node})
}

// renderValue encodes val on its own, giving every value a canonical text used
// to recognize identical values found at different paths.
func renderValue(val *yaml.Node) (string, error) {
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(val); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// yamlMaxSimpleKey is the yaml emitter's limit on simple (inline) mapping
// keys in bytes; longer keys switch to the explicit "? key" form (see
// yaml_emitter_check_simple_key in emitterc.go).
const yamlMaxSimpleKey = 128

// render encodes val as a single "path: value" YAML block.
func render(path string, val *yaml.Node) (string, error) {
	if len(path) > yamlMaxSimpleKey {
		return renderLongKey(path, val)
	}
	doc := &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: path},
			val,
		},
	}
	var sb strings.Builder
	enc := yaml.NewEncoder(&sb)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// renderLongKey keeps the documented one-line "path: value" shape for paths
// beyond the emitter's simple-key limit (long merged alternations get there
// easily): the value is encoded under a one-byte placeholder key, which is
// then replaced with the real path.
func renderLongKey(path string, val *yaml.Node) (string, error) {
	block, err := render("k", val)
	if err != nil {
		return "", err
	}
	return quoteKey(path) + strings.TrimPrefix(block, "k"), nil
}

// quoteKey returns path formatted as a YAML mapping key: verbatim when every
// byte is unambiguously plain-safe, single-quoted otherwise.
func quoteKey(path string) string {
	for i := 0; i < len(path); i++ {
		switch c := path[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '/' || c == '(' || c == ')':
		case (c == '-' || c == '|') && i > 0:
			// safe mid-scalar, but as a first byte "- " or "|" would read as
			// a sequence entry or a block scalar
		default:
			return "'" + strings.ReplaceAll(path, "'", "''") + "'"
		}
	}
	return path
}
