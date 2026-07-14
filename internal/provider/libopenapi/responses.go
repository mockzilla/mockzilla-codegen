// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
)

const statusDefault = "default"

// Response keys sort by rank: exact codes, then ranges like 2XX, then anything else, then default.
const (
	rankCode = iota
	rankRange
	rankOther
	rankDefault
)

var (
	statusCode  = regexp.MustCompile(`^[1-5][0-9][0-9]$`)
	statusRange = regexp.MustCompile(`^[1-5][xX][xX]$`)
)

func (c *converter) responses(r *v3.Responses, ptr string) []*spec.Response {
	if r == nil {
		return nil
	}

	var out []*spec.Response
	for code, resp := range r.Codes.FromOldest() {
		at := ptr + "/" + oasdoc.Escape(code)
		if statusRank(code) == rankOther {
			c.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeInvalidStatus,
				Pointer:  at,
				Origin:   c.position(at, resp.GoLow().GetRootNode()),
				Message:  "response key " + strconv.Quote(code) + " is not a status code, a range or default",
			})
		}
		out = append(out, c.response(resp, code, at))
	}
	if r.Default != nil {
		out = append(out, c.response(r.Default, statusDefault, ptr+"/"+statusDefault))
	}

	slices.SortStableFunc(out, func(a, b *spec.Response) int {
		return cmp.Or(cmp.Compare(statusRank(a.Status), statusRank(b.Status)), cmp.Compare(a.Status, b.Status))
	})
	return out
}

func (c *converter) response(r *v3.Response, status, ptr string) *spec.Response {
	out, ref := memoized(c.responseMemo, r.GoLow(), ptr, func(at string) *spec.Response {
		return &spec.Response{
			Description: r.Description,
			Headers:     c.headers(r.Headers, at+"/headers"),
			Contents:    c.contents(r.Content, at+"/content"),
			Extensions:  extensions(r.Extensions),
			Origin:      c.origin(at, r.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	out.Status = status
	return out
}

func (c *converter) headers(m *orderedmap.Map[string, *v3.Header], ptr string) []*spec.Header {
	var out []*spec.Header
	for name, h := range m.FromOldest() {
		out = append(out, c.header(h, name, ptr+"/"+oasdoc.Escape(name)))
	}
	return out
}

// header names a header by its usage key, which can differ from a referenced component's name.
func (c *converter) header(h *v3.Header, name, ptr string) *spec.Header {
	out, ref := memoized(c.headerMemo, h.GoLow(), ptr, func(at string) *spec.Header {
		style := h.Style
		if style == "" {
			style = styleSimple
		}
		return &spec.Header{
			Description: h.Description,
			Required:    h.Required,
			Deprecated:  h.Deprecated,
			Style:       style,
			Explode:     h.Explode,
			Schema:      c.schema(h.Schema, at+"/schema"),
			Contents:    c.contents(h.Content, at+"/content"),
			Extensions:  extensions(h.Extensions),
			Origin:      c.origin(at, h.GoLow().GetRootNode()),
		}
	})
	if ref != nil {
		out.Ref = ref
	}
	out.Name = name
	return out
}

func statusRank(status string) int {
	switch {
	case statusCode.MatchString(status):
		return rankCode
	case statusRange.MatchString(status):
		return rankRange
	case status == statusDefault:
		return rankDefault
	default:
		return rankOther
	}
}
