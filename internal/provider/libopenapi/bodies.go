// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"

	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

func (c *converter) requestBody(b *v3.RequestBody, ptr string) *spec.RequestBody {
	if b == nil {
		return nil
	}

	out, ref := memoized(c.bodyMemo, b.GoLow(), ptr, func(at string) *spec.RequestBody {
		return &spec.RequestBody{
			Description: b.Description,
			Required:    b.Required != nil && *b.Required,
			Contents:    c.contents(b.Content, at+"/content"),
			Extensions:  extensions(b.Extensions),
			Origin:      c.origin(at, b.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	return out
}

func (c *converter) contents(m *orderedmap.Map[string, *v3.MediaType], ptr string) []*spec.MediaType {
	var out []*spec.MediaType
	for name, mt := range m.FromOldest() {
		at := ptr + "/" + oasdoc.Escape(name)
		out = append(out, &spec.MediaType{
			Name:       name,
			Schema:     c.schema(mt.Schema, at+"/schema"),
			ItemSchema: c.schema(mt.ItemSchema, at+"/itemSchema"),
			Encodings:  c.encodings(mt.Encoding, at+"/encoding"),
			Extensions: extensions(mt.Extensions),
			Origin:     c.origin(at, mt.GoLow().GetRootNode()),
		})
	}
	return out
}

func (c *converter) encodings(m *orderedmap.Map[string, *v3.Encoding], ptr string) []*spec.Encoding {
	var out []*spec.Encoding
	for name, e := range m.FromOldest() {
		enc := &spec.Encoding{
			Name:          name,
			ContentType:   e.ContentType,
			Headers:       c.headers(e.Headers, ptr+"/"+oasdoc.Escape(name)+"/headers"),
			Style:         e.Style,
			AllowReserved: e.AllowReserved,
		}
		if e.Explode != nil {
			enc.Explode = new(*e.Explode)
		}
		out = append(out, enc)
	}
	return out
}
