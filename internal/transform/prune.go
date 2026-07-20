// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
)

const componentsPrefix = "#/components/"

// schemaIDKeys make a schema reachable in ways a JSON pointer does not show.
var schemaIDKeys = []string{"$id", "$anchor", "$dynamicAnchor"}

type component struct {
	section string
	name    string
}

type pruner struct {
	doc      *oasdoc.Doc
	comps    *yaml.Node
	reached  map[component]bool
	queue    []component
	security map[string]bool
	diags    diag.Collector
}

// Prune removes components that no path or webhook reaches, then empty component maps, in place.
// Security schemes stay when a security requirement names them.
func Prune(doc *oasdoc.Doc) (bool, []diag.Diagnostic) {
	comps := doc.Get("/components")
	if comps == nil || comps.Kind != yaml.MappingNode {
		return false, nil
	}

	p := &pruner{doc: doc, comps: comps, reached: map[component]bool{}, security: map[string]bool{}}
	p.keepSchemaIDs()
	p.scan(doc.Get("/paths"), oasdoc.KindPaths)
	if hooks := doc.Get("/webhooks"); hooks != nil {
		for i := 1; i < len(hooks.Content); i += 2 {
			p.scan(hooks.Content[i], oasdoc.KindPathItem)
		}
	}
	p.requirements(doc.Get("/security"))

	for len(p.queue) > 0 {
		c := p.queue[0]
		p.queue = p.queue[1:]
		p.scan(oasdoc.Child(oasdoc.Child(comps, c.section), c.name), oasdoc.SectionKind(c.section))
	}
	return p.remove(), p.diags.List()
}

// keepSchemaIDs keeps component schemas with $id or an anchor: refs can reach them by those.
func (p *pruner) keepSchemaIDs() {
	schemas := oasdoc.Child(p.comps, "schemas")
	for i := 0; schemas != nil && i+1 < len(schemas.Content); i += 2 {
		name := schemas.Content[i].Value
		ptr := "/components/schemas/" + oasdoc.Escape(name)
		oasdoc.Visit(schemas.Content[i+1], ptr, oasdoc.KindSchema, func(at string, n *yaml.Node, _ oasdoc.Kind) bool {
			found := slices.IndexFunc(schemaIDKeys, func(k string) bool { return oasdoc.Child(n, k) != nil })
			if found < 0 {
				return true
			}
			p.diags.Append(diag.Diagnostic{
				Severity: diag.Info,
				Code:     diag.CodePruneUnsupported,
				Pointer:  at,
				Message:  "schema " + name + " has " + schemaIDKeys[found] + ", which pruning cannot follow, so it stays",
			})
			p.reach(componentsPrefix + "schemas/" + oasdoc.Escape(name))
			return false
		})
	}
}

// scan marks what n, an object of kind k, refers to: every $ref under it, discriminator mappings,
// link operationRefs and the security schemes its operations name.
func (p *pruner) scan(n *yaml.Node, k oasdoc.Kind) {
	if n == nil {
		return
	}

	eachRef(n, p.reach)
	oasdoc.Visit(n, "", k, func(_ string, m *yaml.Node, mk oasdoc.Kind) bool {
		switch mk {
		case oasdoc.KindSchema:
			mapping := oasdoc.Child(oasdoc.Child(m, "discriminator"), "mapping")
			for i := 1; mapping != nil && i < len(mapping.Content); i += 2 {
				p.reach(mappingRef(mapping.Content[i].Value))
			}
		case oasdoc.KindOperation:
			p.requirements(oasdoc.Child(m, "security"))
		case oasdoc.KindLink:
			if op := oasdoc.Child(m, "operationRef"); op != nil {
				p.reach(op.Value)
			}
		default:
		}
		return true
	})
}

func (p *pruner) reach(ref string) {
	if !strings.HasPrefix(ref, componentsPrefix) {
		return
	}
	tokens, err := oasdoc.Split(ref)
	if err != nil || len(tokens) < 3 {
		return
	}

	c := component{section: tokens[1], name: tokens[2]}
	if !p.reached[c] {
		p.reached[c] = true
		p.queue = append(p.queue, c)
	}
}

func (p *pruner) requirements(n *yaml.Node) {
	if n == nil {
		return
	}
	for _, req := range n.Content {
		for i := 0; i+1 < len(req.Content); i += 2 {
			p.security[req.Content[i].Value] = true
		}
	}
}

func (p *pruner) remove() bool {
	isEntryRemoved := false
	isSectionRemoved := oasdoc.DeleteChildren(p.comps, func(section string, m *yaml.Node) bool {
		if oasdoc.SectionKind(section) == oasdoc.KindNone || m.Kind != yaml.MappingNode {
			return false
		}
		if oasdoc.DeleteChildren(m, func(name string, _ *yaml.Node) bool {
			return !p.reached[component{section: section, name: name}] && (section != "securitySchemes" || !p.security[name])
		}) {
			isEntryRemoved = true
		}
		return len(m.Content) == 0
	})

	isChanged := isEntryRemoved || isSectionRemoved
	if isChanged && len(p.comps.Content) == 0 {
		oasdoc.DeleteChild(p.doc.Root(), "components")
	}
	return isChanged
}

// mappingRef turns a bare discriminator mapping name into the schema ref it stands for.
func mappingRef(value string) string {
	if strings.ContainsAny(value, "#/") {
		return value
	}
	return componentsPrefix + "schemas/" + oasdoc.Escape(value)
}

func eachRef(n *yaml.Node, fn func(ref string)) {
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			if v := n.Content[i+1]; n.Content[i].Value == "$ref" && v.Kind == yaml.ScalarNode {
				fn(v.Value)
			}
			eachRef(n.Content[i+1], fn)
		}
	case yaml.SequenceNode:
		for _, c := range n.Content {
			eachRef(c, fn)
		}
	default:
	}
}
