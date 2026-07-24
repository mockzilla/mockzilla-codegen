// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
	"github.com/mockzilla/codegen/pkg/config"
)

type filter struct {
	doc       *oasdoc.Doc
	include   config.FilterSet
	exclude   config.FilterSet
	diags     diag.Collector
	isChanged bool
}

// Filter drops the operations, webhooks, schema properties and schema extensions f leaves out, in
// place. Exclude wins over include, and an empty include list keeps everything.
func Filter(doc *oasdoc.Doc, f config.Filter) (bool, []diag.Diagnostic) {
	fl := &filter{doc: doc, include: f.Include, exclude: f.Exclude}
	fl.operations()
	fl.properties()
	fl.extensions()
	return fl.isChanged, fl.diags.List()
}

func (fl *filter) operations() {
	in, ex := fl.include, fl.exclude
	if len(in.Paths)+len(ex.Paths)+len(in.Tags)+len(ex.Tags)+len(in.OperationIDs)+len(ex.OperationIDs)+
		len(in.Webhooks)+len(ex.Webhooks) == 0 {
		return
	}

	hadOperations := HasOperations(fl.doc)
	fl.items("paths", in.Paths, ex.Paths)
	fl.items("webhooks", in.Webhooks, ex.Webhooks)
	if hadOperations && !HasOperations(fl.doc) {
		fl.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeFilterEmpty,
			Message:  "the filter removed every operation",
		})
	}
}

// items filters the paths or webhooks map; a path item left with no operations goes too.
func (fl *filter) items(section string, include, exclude []string) {
	m := fl.doc.Get("/" + section)
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}

	isRemoved := oasdoc.DeleteChildren(m, func(key string, item *yaml.Node) bool {
		if (len(include) > 0 && !slices.Contains(include, key)) || slices.Contains(exclude, key) {
			return true
		}
		return (section != "paths" || !strings.HasPrefix(key, "x-")) && fl.item(item, key)
	})
	fl.isChanged = fl.isChanged || isRemoved
}

// item drops the operations of path item n that the filter leaves out and reports whether none
// are left. A path item behind a $ref gets a copy of its target first.
func (fl *filter) item(n *yaml.Node, path string) bool {
	item := resolveLocal(fl.doc, n)
	ops := operationsOf(item)
	var drop []operation
	for _, op := range ops {
		if !fl.keeps(op, path) {
			drop = append(drop, op)
		}
	}
	switch {
	case len(drop) == 0:
		return false
	case len(drop) == len(ops):
		return true
	}

	if item != n {
		copied := oasdoc.Clone(item)
		for j := 0; j+1 < len(n.Content); j += 2 {
			if key := n.Content[j].Value; key != "$ref" {
				oasdoc.SetChild(copied, key, n.Content[j+1])
			}
		}
		n.Content, item = copied.Content, n
	}
	for _, op := range drop {
		if !op.isAdditional {
			oasdoc.DeleteChild(item, op.method)
			continue
		}
		if extra := oasdoc.Child(item, additionalOperations); extra != nil {
			oasdoc.DeleteChild(extra, op.method)
			if len(extra.Content) == 0 {
				oasdoc.DeleteChild(item, additionalOperations)
			}
		}
	}
	fl.isChanged = true
	return false
}

func (fl *filter) keeps(op operation, path string) bool {
	id := spec.DeriveOperationID(op.method, path)
	if n := oasdoc.Child(op.node, "operationId"); n != nil && n.Value != "" {
		id = n.Value
	}
	tags := scalars(oasdoc.Child(op.node, "tags"))
	hasTag := func(list []string) bool {
		return slices.ContainsFunc(tags, func(t string) bool { return slices.Contains(list, t) })
	}

	switch {
	case hasTag(fl.exclude.Tags), slices.Contains(fl.exclude.OperationIDs, id):
		return false
	case len(fl.include.Tags) > 0 && !hasTag(fl.include.Tags):
		return false
	case len(fl.include.OperationIDs) > 0 && !slices.Contains(fl.include.OperationIDs, id):
		return false
	default:
		return true
	}
}

// properties applies schema-properties: required properties always stay.
func (fl *filter) properties() {
	names := slices.Sorted(maps.Keys(fl.include.SchemaProperties))
	names = append(names, slices.Sorted(maps.Keys(fl.exclude.SchemaProperties))...)
	for i, name := range names {
		isInclude := i < len(fl.include.SchemaProperties)
		ptr := "/components/schemas/" + oasdoc.Escape(name)
		schema := fl.doc.Get(ptr)
		if schema == nil {
			fl.warn(diag.CodeFilterUnknown, ptr, "schema "+name+" in schema-properties does not exist")
			continue
		}

		listed := fl.exclude.SchemaProperties[name]
		if isInclude {
			listed = fl.include.SchemaProperties[name]
		}
		fl.schemaProperties(ptr, schema, listed, isInclude)
	}
}

func (fl *filter) schemaProperties(ptr string, schema *yaml.Node, listed []string, isInclude bool) {
	props := oasdoc.Child(schema, "properties")
	required := scalars(oasdoc.Child(schema, "required"))
	for _, p := range listed {
		if oasdoc.Child(props, p) == nil {
			fl.warn(diag.CodeFilterUnknown, ptr+"/properties/"+oasdoc.Escape(p), "property "+p+" in schema-properties does not exist")
		}
	}

	if props == nil || props.Kind != yaml.MappingNode {
		return
	}
	isRemoved := oasdoc.DeleteChildren(props, func(name string, _ *yaml.Node) bool {
		isRequired := slices.Contains(required, name)
		isDropped := slices.Contains(listed, name) != isInclude
		if isDropped && isRequired {
			fl.warn(diag.CodeFilterRequired, ptr+"/properties/"+oasdoc.Escape(name), "property "+name+" is required, so it stays")
		}
		return isDropped && !isRequired
	})
	fl.isChanged = fl.isChanged || isRemoved
}

// extensions keeps or drops x- keys on component schemas.
func (fl *filter) extensions() {
	in, ex := fl.include.Extensions, fl.exclude.Extensions
	schemas := fl.doc.Get("/components/schemas")
	if len(in)+len(ex) == 0 || schemas == nil {
		return
	}

	drop := func(key string, _ *yaml.Node) bool {
		return strings.HasPrefix(key, "x-") && ((len(in) > 0 && !slices.Contains(in, key)) || slices.Contains(ex, key))
	}
	for i := 1; i < len(schemas.Content); i += 2 {
		if s := schemas.Content[i]; s.Kind == yaml.MappingNode && oasdoc.DeleteChildren(s, drop) {
			fl.isChanged = true
		}
	}
}

func (fl *filter) warn(code, ptr, msg string) {
	fl.diags.Append(diag.Diagnostic{Severity: diag.Warning, Code: code, Pointer: ptr, Message: msg})
}
