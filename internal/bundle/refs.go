// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// $refs and components as YAML: refs to other files, the components of a document, and the name
// a ref target gives.

package bundle

import (
	"path"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
)

func hasExternalRef(d *oasdoc.Doc) bool {
	for _, r := range d.Refs() {
		if !strings.HasPrefix(r.Value, "#") {
			return true
		}
	}
	return false
}

func sectionEntries(comps *yaml.Node) []entry {
	var out []entry
	for i := 0; comps != nil && i+1 < len(comps.Content); i += 2 {
		section, m := comps.Content[i].Value, comps.Content[i+1]
		if oasdoc.SectionKind(section) == oasdoc.KindNone || m.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(m.Content); j += 2 {
			out = append(out, entry{section: section, name: m.Content[j].Value, value: m.Content[j+1]})
		}
	}
	return out
}

// pureRef is the $ref of an object that holds nothing else, or "".
func pureRef(n *yaml.Node) string {
	if n.Kind != yaml.MappingNode || len(n.Content) != 2 || n.Content[0].Value != refKey || n.Content[1].Kind != yaml.ScalarNode {
		return ""
	}
	return n.Content[1].Value
}

func componentRef(section, name string) string {
	return "#/components/" + section + "/" + oasdoc.Escape(name)
}

// nameOf is the last pointer token of t, or the file name for a whole document.
func nameOf(t target) string {
	tokens, err := oasdoc.Split("#" + t.frag)
	if err != nil || len(tokens) == 0 || tokens[len(tokens)-1] == "" {
		return stem(t.loc)
	}
	return tokens[len(tokens)-1]
}

func isFileName(s string) bool {
	switch path.Ext(s) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}
