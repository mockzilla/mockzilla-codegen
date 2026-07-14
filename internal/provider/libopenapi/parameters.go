// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"slices"
	"strconv"

	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/spec"
)

const (
	styleSimple = "simple"
	styleForm   = "form"
)

func (c *converter) parameters(list []*v3.Parameter, ptr string) []*spec.Parameter {
	var out []*spec.Parameter
	for i, p := range list {
		out = append(out, c.parameter(p, ptr+"/"+strconv.Itoa(i)))
	}
	return out
}

func (c *converter) parameter(p *v3.Parameter, ptr string) *spec.Parameter {
	out, ref := memoized(c.paramMemo, p.GoLow(), ptr, func(at string) *spec.Parameter { return c.buildParameter(p, at) })
	if ref != nil {
		out.Ref = ref
	}
	return out
}

// buildParameter fills in the default style and explode, and treats a path parameter as required.
func (c *converter) buildParameter(p *v3.Parameter, ptr string) *spec.Parameter {
	out := &spec.Parameter{
		Name:            p.Name,
		In:              p.In,
		Description:     p.Description,
		Required:        p.Required != nil && *p.Required,
		Deprecated:      p.Deprecated,
		AllowEmptyValue: p.AllowEmptyValue,
		Style:           p.Style,
		AllowReserved:   p.AllowReserved,
		Schema:          c.schema(p.Schema, ptr+"/schema"),
		Contents:        c.contents(p.Content, ptr+"/content"),
		Extensions:      extensions(p.Extensions),
		Origin:          c.origin(ptr, p.GoLow().GetRootNode()),
	}

	if out.Style == "" {
		switch out.In {
		case spec.InQuery, spec.InCookie:
			out.Style = styleForm
		case spec.InPath, spec.InHeader:
			out.Style = styleSimple
		}
	}
	out.Explode = out.Style == styleForm
	if p.Explode != nil {
		out.Explode = *p.Explode
	}

	if out.In == spec.InPath && !out.Required {
		out.Required = true
		c.diags.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeOptionalPathParam,
			Pointer:  ptr,
			Origin:   c.position(ptr, p.GoLow().GetRootNode()),
			Message:  "path parameter " + out.Name + " is not marked required; treating it as required",
		})
	}
	return out
}

func mergeParameters(shared, own []*spec.Parameter) []*spec.Parameter {
	if len(shared) == 0 {
		return own
	}

	out := slices.Clone(shared)
	for _, p := range own {
		i := slices.IndexFunc(out[:len(shared)], func(s *spec.Parameter) bool { return s.In == p.In && s.Name == p.Name })
		if i < 0 {
			out = append(out, p)
			continue
		}
		out[i] = p
	}
	return out
}
