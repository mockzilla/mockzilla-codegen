// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"slices"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"

	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

var lowerMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace", "query"}

// site is where an operation sits; path is a webhook name or callback expression for those.
type site struct {
	method    string
	path      string
	ptr       string
	isWebhook bool
	shared    []*spec.Parameter
}

func (c *converter) paths(p *v3.Paths) []*spec.Operation {
	if p == nil {
		return nil
	}

	var out []*spec.Operation
	for path, item := range p.PathItems.FromOldest() {
		out = append(out, c.pathItem(item, site{path: path, ptr: "/paths/" + oasdoc.Escape(path)})...)
	}
	return out
}

func (c *converter) webhooks(m *orderedmap.Map[string, *v3.PathItem]) []*spec.Operation {
	var out []*spec.Operation
	for name, item := range m.FromOldest() {
		out = append(out, c.pathItem(item, site{path: name, ptr: "/webhooks/" + oasdoc.Escape(name), isWebhook: true})...)
	}
	return out
}

// pathItem takes at.ptr as the path item's pointer.
func (c *converter) pathItem(item *v3.PathItem, at site) []*spec.Operation {
	itemPtr := at.ptr
	at.shared = c.parameters(item.Parameters, itemPtr+"/parameters")

	fixed := []*v3.Operation{item.Get, item.Put, item.Post, item.Delete, item.Options, item.Head, item.Patch, item.Trace, item.Query}
	var out []*spec.Operation
	for i, op := range fixed {
		if op == nil {
			continue
		}
		at.method = spec.MethodOrder[i]
		at.ptr = itemPtr + "/" + lowerMethods[i]
		out = append(out, c.operation(op, at))
	}
	// libopenapi v0.38.7 leaves the high-level AdditionalOperations empty, so read the low model.
	for k, v := range item.GoLow().AdditionalOperations.Value.FromOldest() {
		at.method = k.Value
		at.ptr = itemPtr + "/additionalOperations/" + oasdoc.Escape(k.Value)
		out = append(out, c.operation(v3.NewOperation(v.Value), at))
	}
	return out
}

func (c *converter) operation(o *v3.Operation, at site) *spec.Operation {
	low := o.GoLow()
	out := &spec.Operation{
		ID:          o.OperationId,
		Method:      at.method,
		Path:        at.path,
		IsWebhook:   at.isWebhook,
		Summary:     o.Summary,
		Description: o.Description,
		Deprecated:  o.Deprecated != nil && *o.Deprecated,
		Tags:        slices.Clone(o.Tags),
		Params:      mergeParameters(at.shared, c.parameters(o.Parameters, at.ptr+"/parameters")),
		Body:        c.requestBody(o.RequestBody, at.ptr+"/requestBody"),
		Responses:   c.responses(o.Responses, at.ptr+"/responses"),
		Callbacks:   c.callbacks(o.Callbacks, at.ptr+"/callbacks"),
		Security:    c.security,
		Servers:     servers(o.Servers),
		Extensions:  extensions(o.Extensions),
		Origin:      c.origin(at.ptr, low.GetRootNode()),
	}
	if low.Security.KeyNode != nil {
		out.Security = securityRequirements(o.Security)
	}
	if out.ID == "" {
		out.ID, out.IsIDDerived = spec.DeriveOperationID(at.method, at.path), true
	}
	return out
}

// callbacks converts a referenced callback at its target so its schemas are shared.
func (c *converter) callbacks(m *orderedmap.Map[string, *v3.Callback], ptr string) []*spec.Callback {
	var out []*spec.Callback
	for name, cb := range m.FromOldest() {
		cbPtr := ptr + "/" + oasdoc.Escape(name)
		item := &spec.Callback{Name: name}
		if low := cb.GoLow(); low.IsReference() {
			cbPtr = refPointer(low.GetReference())
			item.Ref = componentRef(cbPtr)
		}
		for expr, pi := range cb.Expression.FromOldest() {
			item.Operations = append(item.Operations, c.pathItem(pi, site{path: expr, ptr: cbPtr + "/" + oasdoc.Escape(expr)})...)
		}
		out = append(out, item)
	}
	return out
}
