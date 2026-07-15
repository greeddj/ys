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

	"gopkg.in/yaml.v3"
)

// collectFiles gathers the unique YAML files reachable from roots, walking
// directories recursively. Unreadable roots are reported to errw and skipped;
// the returned list is sorted for stable output.
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
	sort.Strings(files)
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
// dotted path matches.
func scanFile(file string, match matcher) ([]hit, error) {
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
		stripComments(&doc)
		walk(&doc, nil, match, &hits)
	}
	return hits, nil
}

// stripComments clears comments recursively so that two values that differ only
// by an incidental comment are treated (and printed) as the same value.
func stripComments(n *yaml.Node) {
	n.HeadComment, n.LineComment, n.FootComment = "", "", ""
	for _, c := range n.Content {
		stripComments(c)
	}
}

// walk visits every node, recording a hit whenever its dotted path matches.
func walk(n *yaml.Node, path []string, match matcher, out *[]hit) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			walk(c, path, match, out)
		}
	case yaml.MappingNode:
		check(n, path, match, out)
		for i := 0; i+1 < len(n.Content); i += 2 {
			walk(n.Content[i+1], childPath(path, n.Content[i].Value), match, out)
		}
	case yaml.SequenceNode:
		check(n, path, match, out)
		for i, c := range n.Content {
			walk(c, childPath(path, strconv.Itoa(i)), match, out)
		}
	case yaml.ScalarNode:
		check(n, path, match, out)
	}
}

// childPath returns a fresh slice so sibling recursion never shares backing storage.
func childPath(path []string, seg string) []string {
	next := make([]string, len(path)+1)
	copy(next, path)
	next[len(path)] = seg
	return next
}

// check appends a hit when the node at path matches and renders cleanly.
func check(n *yaml.Node, path []string, match matcher, out *[]hit) {
	if len(path) == 0 {
		return
	}
	dotted := strings.Join(path, ".")
	if !match(dotted, path[len(path)-1]) {
		return
	}
	block, err := render(dotted, n)
	if err != nil {
		return
	}
	*out = append(*out, hit{path: dotted, block: block})
}

// render encodes val as a single "path: value" YAML block.
func render(path string, val *yaml.Node) (string, error) {
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
