// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Whether a default fits its schema, checked against what the document writes of the schema the
// way the MCP SDK checks every default when a tool is added.

package jsonschema

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// maxShown is the longest default a warning quotes.
const maxShown = 40

// misfit returns why the default v does not fit s, or "" when it fits.
func misfit(v spec.Value, s *spec.Schema) string {
	if !isReadable(v) {
		return "it holds a number too large for a float64"
	}
	return mismatch(v, s, "", map[*spec.Schema]bool{})
}

// mismatch returns why v, at the pointer at in the default, does not fit s, or "" when it fits.
// on holds the schemas being checked against v, so a $ref cycle that never goes into v ends.
func mismatch(v spec.Value, s *spec.Schema, at string, on map[*spec.Schema]bool) string {
	if s == nil || on[s] {
		return ""
	}
	on[s] = true
	defer delete(on, s)

	if s.Nullable && v.Kind == spec.KindNull {
		return ""
	}
	if s.Ref != nil {
		if why := mismatch(v, s.Ref.Target, at, on); why != "" {
			return why
		}
	}
	if why := valueMismatch(v, s, at); why != "" {
		return why
	}
	if why := compositionMismatch(v, s, at, on); why != "" {
		return why
	}
	if v.Kind == spec.KindArray {
		return arrayMismatch(v.Items, s, at)
	}
	if v.Kind == spec.KindObject {
		return objectMismatch(v.Fields, s, at)
	}
	return ""
}

// valueMismatch checks the keywords on v itself: type, enum, const, the bounds of a number and the
// length of a string.
func valueMismatch(v spec.Value, s *spec.Schema, at string) string {
	got := jsonType(v)
	want := typeList(s)
	if len(want) > 0 && !slices.Contains(want, got) && (got != "integer" || !slices.Contains(want, "number")) {
		return where(at) + " is " + article(got) + ", the schema wants " + strings.Join(want, " or ")
	}
	if len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(e spec.Value) bool { return isEqual(e, v) }) {
		return where(at) + " is none of the enum values"
	}
	if s.Const != nil && !isEqual(*s.Const, v) {
		return where(at) + " is not the const value"
	}

	switch v.Kind {
	case spec.KindNumber:
		return numberMismatch(v.Num, s.Limits, at)
	case spec.KindString:
		return lengthMismatch(v.Str, s.Limits, at)
	case spec.KindNull, spec.KindBool, spec.KindArray, spec.KindObject:
	}
	return ""
}

// numberMismatch checks n against multipleOf and the bounds, in float64 as the SDK does.
func numberMismatch(n json.Number, l spec.Limits, at string) string {
	f := asFloat(n)
	if l.MultipleOf != nil {
		if _, frac := math.Modf(f / asFloat(*l.MultipleOf)); frac != 0 {
			return where(at) + " is no multiple of " + l.MultipleOf.String()
		}
	}
	if b := l.Minimum; b != nil {
		switch m := asFloat(b.Value); {
		case b.Exclusive && f <= m:
			return where(at) + " is not above the exclusive minimum " + b.Value.String()
		case f < m:
			return where(at) + " is below the minimum " + b.Value.String()
		}
	}
	if b := l.Maximum; b != nil {
		switch m := asFloat(b.Value); {
		case b.Exclusive && f >= m:
			return where(at) + " is not below the exclusive maximum " + b.Value.String()
		case f > m:
			return where(at) + " is above the maximum " + b.Value.String()
		}
	}
	return ""
}

// lengthMismatch checks the length of str, in code points.
func lengthMismatch(str string, l spec.Limits, at string) string {
	n := int64(utf8.RuneCountInString(str))
	if l.MinLength != nil && n < *l.MinLength {
		return where(at) + " is shorter than " + strconv.FormatInt(*l.MinLength, 10) + " characters"
	}
	if l.MaxLength != nil && n > *l.MaxLength {
		return where(at) + " is longer than " + strconv.FormatInt(*l.MaxLength, 10) + " characters"
	}
	return ""
}

// compositionMismatch checks allOf, anyOf, oneOf, not and if, then and else.
func compositionMismatch(v spec.Value, s *spec.Schema, at string, on map[*spec.Schema]bool) string {
	for _, sub := range s.AllOf {
		if why := mismatch(v, sub, at, on); why != "" {
			return why
		}
	}
	isFit := func(sub *spec.Schema) bool { return mismatch(v, sub, at, on) == "" }
	if len(s.AnyOf) > 0 && !slices.ContainsFunc(s.AnyOf, isFit) {
		return where(at) + " fits none of anyOf"
	}
	if len(s.OneOf) > 0 {
		switch n := countFunc(s.OneOf, isFit); {
		case n == 0:
			return where(at) + " fits none of oneOf"
		case n > 1:
			return where(at) + " fits more than one of oneOf"
		}
	}
	if s.Not != nil && isFit(s.Not) {
		return where(at) + " fits the schema of not"
	}
	if s.If == nil {
		return ""
	}
	if isFit(s.If) {
		return mismatch(v, s.Then, at, on)
	}
	return mismatch(v, s.Else, at, on)
}

// arrayMismatch checks the items, their count and uniqueItems.
func arrayMismatch(items []spec.Value, s *spec.Schema, at string) string {
	for i, item := range items {
		sub := s.Items
		if i < len(s.PrefixItems) {
			sub = s.PrefixItems[i]
		}
		if why := mismatch(item, sub, at+"/"+strconv.Itoa(i), map[*spec.Schema]bool{}); why != "" {
			return why
		}
	}

	n, l := int64(len(items)), s.Limits
	if l.MinItems != nil && n < *l.MinItems {
		return where(at) + " has fewer than " + strconv.FormatInt(*l.MinItems, 10) + " items"
	}
	if l.MaxItems != nil && n > *l.MaxItems {
		return where(at) + " has more than " + strconv.FormatInt(*l.MaxItems, 10) + " items"
	}
	for i := range items {
		if l.UniqueItems && slices.ContainsFunc(items[:i], func(x spec.Value) bool { return isEqual(x, items[i]) }) {
			return where(at) + " has item " + strconv.Itoa(i) + " twice"
		}
	}
	return ""
}

// objectMismatch checks the properties, additionalProperties, the property count and required.
func objectMismatch(fields []spec.Field, s *spec.Schema, at string) string {
	for _, f := range fields {
		var sub *spec.Schema
		i := slices.IndexFunc(s.Properties, func(p *spec.Property) bool { return p.Name == f.Name })
		switch add := s.AdditionalProperties; {
		case i >= 0:
			sub = s.Properties[i].Schema
		case add.Mode == spec.AdditionalDenied:
			return where(at) + " has the property " + strconv.Quote(f.Name) + ", which the schema does not allow"
		case add.Mode == spec.AdditionalSchema:
			sub = add.Schema
		}
		if why := mismatch(f.Value, sub, at+"/"+escapePointer(f.Name), map[*spec.Schema]bool{}); why != "" {
			return why
		}
	}

	n, l := int64(len(fields)), s.Limits
	if l.MinProperties != nil && n < *l.MinProperties {
		return where(at) + " has fewer than " + strconv.FormatInt(*l.MinProperties, 10) + " properties"
	}
	if l.MaxProperties != nil && n > *l.MaxProperties {
		return where(at) + " has more than " + strconv.FormatInt(*l.MaxProperties, 10) + " properties"
	}
	for _, name := range s.Required {
		if !slices.ContainsFunc(fields, func(f spec.Field) bool { return f.Name == name }) {
			return where(at) + " lacks the required property " + strconv.Quote(name)
		}
	}
	return ""
}

// jsonType is the JSON type of v as the SDK sees it: a number without a fraction is an integer.
func jsonType(v spec.Value) string {
	switch v.Kind {
	case spec.KindString:
		return "string"
	case spec.KindNumber:
		if _, frac := math.Modf(asFloat(v.Num)); frac == 0 {
			return "integer"
		}
		return "number"
	case spec.KindBool:
		return "boolean"
	case spec.KindArray:
		return "array"
	case spec.KindObject:
		return "object"
	case spec.KindNull:
	}
	return "null"
}

// isEqual compares two JSON values, numbers by their float64 value and objects by their fields
// in any order.
func isEqual(a, b spec.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case spec.KindString:
		return a.Str == b.Str
	case spec.KindNumber:
		return asFloat(a.Num) == asFloat(b.Num)
	case spec.KindBool:
		return a.Bool == b.Bool
	case spec.KindArray:
		return slices.EqualFunc(a.Items, b.Items, isEqual)
	case spec.KindObject:
		return len(a.Fields) == len(b.Fields) && !slices.ContainsFunc(a.Fields, func(f spec.Field) bool {
			i := slices.IndexFunc(b.Fields, func(g spec.Field) bool { return g.Name == f.Name })
			return i < 0 || !isEqual(f.Value, b.Fields[i].Value)
		})
	case spec.KindNull:
	}
	return true
}

// isReadable reports whether every number in v fits a float64.
func isReadable(v spec.Value) bool {
	switch v.Kind {
	case spec.KindNumber:
		_, err := strconv.ParseFloat(string(v.Num), 64)
		return err == nil
	case spec.KindArray:
		return !slices.ContainsFunc(v.Items, func(x spec.Value) bool { return !isReadable(x) })
	case spec.KindObject:
		return !slices.ContainsFunc(v.Fields, func(f spec.Field) bool { return !isReadable(f.Value) })
	case spec.KindNull, spec.KindString, spec.KindBool:
	}
	return true
}

// shown is v written compact as JSON when it is short enough to quote, else "".
func shown(v spec.Value) string {
	text := appendValue(nil, Value(v))
	if len(text) > maxShown {
		return ""
	}
	return string(text)
}

// asFloat is n as a float64; isReadable turned away a default with a number too large for one.
func asFloat(n json.Number) float64 {
	f, _ := strconv.ParseFloat(string(n), 64)
	return f
}

func countFunc(list []*spec.Schema, f func(*spec.Schema) bool) int {
	n := 0
	for _, s := range list {
		if f(s) {
			n++
		}
	}
	return n
}

func where(at string) string {
	if at == "" {
		return "it"
	}
	return at
}

func article(name string) string {
	switch name {
	case "null":
		return "null"
	case "integer", "object", "array":
		return "an " + name
	}
	return "a " + name
}
