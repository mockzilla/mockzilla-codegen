// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Which parameters the runtime can write, by the OpenAPI style table and the depth of the value.

package gomodel

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// paramShape is a parameter's value: one value, a list, an object, or a type not looked into.
type paramShape int

const (
	paramUnknown paramShape = iota
	paramValue
	paramList
	paramObject
)

const (
	styleForm   = "form"
	styleCookie = "cookie"
	styleDeep   = "deepObject"
)

// stylesIn are the styles OpenAPI allows in each location.
var stylesIn = map[string][]string{
	spec.InPath:   {"matrix", "label", "simple"},
	spec.InQuery:  {styleForm, "spaceDelimited", "pipeDelimited", styleDeep},
	spec.InHeader: {"simple"},
	spec.InCookie: {styleForm, styleCookie},
}

// paramIssue says why p cannot be written, so it gets no field, or "" when it can.
func (c *collector) paramIssue(p *spec.Parameter) string {
	if p.Schema == nil {
		return ""
	}
	if p.In != spec.InPath && c.isUnionLost(p.Schema, map[*spec.Schema]bool{}) {
		return "is or holds a union of more than scalars"
	}
	sh, f := c.paramShapeOf(p.Schema)
	if why := styleIssue(p, sh); why != "" {
		return why
	}
	switch sh {
	case paramList:
		if item, _ := c.paramShapeOf(f.Items); item == paramList || item == paramObject {
			return "is a list of " + plural(item) + ", which OpenAPI leaves to the implementation"
		}
	case paramObject:
		isRepeated := p.Explode && (p.Style == styleForm || p.Style == styleCookie)
		return c.propertyIssue(f, p.Style == styleDeep, isRepeated, map[*spec.Schema]bool{})
	case paramUnknown, paramValue:
	}
	return ""
}

// paramShapeOf is the shape of s as a parameter, and s flattened.
func (c *collector) paramShapeOf(s *spec.Schema) (paramShape, *spec.Schema) {
	if s == nil || c.flat.goTypeOf(s) != nil {
		return paramUnknown, nil
	}
	f := c.flat.flatten(target(s))
	switch classify(f) {
	case shapePrimitive:
		if _, isMapped := c.formats[strings.ToLower(f.Format)]; isMapped {
			return paramUnknown, f
		}
		return paramValue, f
	case shapeEnum:
		return paramValue, f
	case shapeUnion:
		if c.isScalar(s, map[*spec.Schema]bool{}) {
			return paramValue, f
		}
	case shapeArray:
		return paramList, f
	case shapeStruct, shapeMap:
		return paramObject, f
	case shapeAny:
	}
	return paramUnknown, f
}

// propertyIssue says why a property of f is nested deeper than its style writes, or "" if none.
func (c *collector) propertyIssue(f *spec.Schema, isDeep, isRepeated bool, on map[*spec.Schema]bool) string {
	if on[f] {
		return ""
	}
	on[f] = true
	defer delete(on, f)

	for _, p := range objectValues(f) {
		sh, inner := c.paramShapeOf(p.Schema)
		where := "its map values"
		if p.Name != "" {
			where = fmt.Sprintf("property %q", p.Name)
		}
		switch {
		case sh == paramObject && isDeep:
			if why := c.propertyIssue(inner, true, false, on); why != "" {
				return why
			}
		case sh == paramObject:
			return "holds an object in " + where + ", which OpenAPI leaves to the implementation"
		case sh == paramList && !isDeep && !isRepeated:
			return "holds a list in " + where + ", which OpenAPI leaves to the implementation"
		case sh == paramList:
			if item, _ := c.paramShapeOf(inner.Items); item == paramList || item == paramObject {
				return "holds a list of " + plural(item) + " in " + where + ", which OpenAPI leaves to the implementation"
			}
		}
	}
	return ""
}

// styleIssue says why the style of p does not fit its location or a value of shape sh.
func styleIssue(p *spec.Parameter, sh paramShape) string {
	if !slices.Contains(stylesIn[p.In], p.Style) {
		var where []string
		for _, in := range paramOrder {
			if slices.Contains(stylesIn[in], p.Style) {
				where = append(where, in)
			}
		}
		if len(where) == 0 {
			return fmt.Sprintf("has style %q, which OpenAPI does not define", p.Style)
		}
		return fmt.Sprintf("has style %s, which OpenAPI allows only in %s parameters", p.Style, strings.Join(where, " and "))
	}

	isDelimited := p.Style == "spaceDelimited" || p.Style == "pipeDelimited"
	switch {
	case isDelimited && sh == paramValue:
		return fmt.Sprintf("has style %s, which OpenAPI defines for a list or an object only", p.Style)
	case isDelimited && p.Explode:
		return fmt.Sprintf("has style %s with explode true, which OpenAPI does not define", p.Style)
	case p.Style == styleDeep && (sh == paramValue || sh == paramList):
		return "has style deepObject, which OpenAPI defines for an object only"
	case p.In == spec.InCookie && p.Style == styleForm && p.Explode && (sh == paramList || sh == paramObject):
		return "has style form with explode true, which OpenAPI says writes the wrong delimiter for several cookie values (use style cookie)"
	}
	return ""
}

// objectValues are the properties of the object f, then its additional properties under no name.
func objectValues(f *spec.Schema) []*spec.Property {
	out := slices.Clone(f.Properties)
	if f.AdditionalProperties.Mode == spec.AdditionalSchema {
		out = append(out, &spec.Property{Schema: f.AdditionalProperties.Schema})
	}
	return out
}

func plural(sh paramShape) string {
	if sh == paramList {
		return "lists"
	}
	return "objects"
}
