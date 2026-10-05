// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Schema keywords read from their nodes as written, and the warnings about what is wrong in them.

package libopenapi

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// unsupportedKeywords are the schema keywords no generated type checks.
var unsupportedKeywords = []string{
	"patternProperties", "prefixItems", "not", "contains", "minContains", "maxContains",
	"dependentRequired", "dependentSchemas", "unevaluatedProperties", "unevaluatedItems",
}

// keywords reads the keywords of the schema at ptr from its mapping node.
type keywords struct {
	c    *converter
	node *yaml.Node
	ptr  string
}

// limits reads every number from its node, so it keeps the precision and form it was written in.
func (k keywords) limits() spec.Limits {
	out := spec.Limits{
		Minimum:       boundSource{value: k.number("minimum"), exclusive: k.exclusive("exclusiveMinimum")}.resolve(true),
		Maximum:       boundSource{value: k.number("maximum"), exclusive: k.exclusive("exclusiveMaximum")}.resolve(false),
		MinLength:     k.count("minLength"),
		MaxLength:     k.count("maxLength"),
		MinItems:      k.count("minItems"),
		MaxItems:      k.count("maxItems"),
		MinProperties: k.count("minProperties"),
		MaxProperties: k.count("maxProperties"),
		UniqueItems:   k.flag("uniqueItems"),
	}
	if v := k.read("multipleOf", "a number above 0", isPositive); v != nil {
		out.MultipleOf = &v.Num
	}
	return out
}

func (k keywords) number(key string) *spec.Value {
	return k.read(key, "a number", isNumber)
}

func (k keywords) count(key string) *int64 {
	v := k.read(key, "a whole number of 0 or more", isCount)
	if v == nil {
		return nil
	}
	n, _ := v.Num.Int64()
	return &n
}

func (k keywords) flag(key string) bool {
	v := k.read(key, "true or false", isBool)
	return v != nil && v.Bool
}

// exclusive reads exclusiveMinimum or exclusiveMaximum in the form its value has, in any version.
func (k keywords) exclusive(key string) *spec.Value {
	v := k.read(key, "a number, true or false", func(v spec.Value) bool { return isNumber(v) || isBool(v) })
	switch {
	case v == nil:
	case isBool(*v) && k.c.version != spec.V30:
		k.warn(diag.CodeKeywordVersion, key, fmt.Sprintf("%s %t is the 3.0 form; a %s spec takes a number", key, v.Bool, k.c.version))
	case isNumber(*v) && k.c.version == spec.V30:
		k.warn(diag.CodeKeywordVersion, key, fmt.Sprintf("%s %s is the 3.1 form; a 3.0 spec takes true or false", key, v.Num))
	}
	return v
}

// read returns the value under key when fits accepts it, and reports one it does not as left out.
func (k keywords) read(key, want string, fits func(spec.Value) bool) *spec.Value {
	n := oasdoc.Child(k.node, key)
	if n == nil {
		return nil
	}
	v := value(n)
	if fits(v) {
		return &v
	}
	k.warn(diag.CodeKeywordInvalid, key, fmt.Sprintf("%s must be %s, not %s; it is left out", key, want, written(v, n)))
	return nil
}

// unsupported reports each keyword of the schema that no generated type checks.
func (k keywords) unsupported() {
	for i := 0; i+1 < len(k.node.Content); i += 2 {
		if key := k.node.Content[i].Value; slices.Contains(unsupportedKeywords, key) {
			k.warn(diag.CodeKeywordUnsupported, key, key+" is not supported; generated types do not check it")
		}
	}
}

func (k keywords) warn(code, key, msg string) {
	k.c.warn(code, k.ptr+"/"+key, oasdoc.Child(k.node, key), msg)
}

// boundSource is one side of a range: the plain keyword and the exclusive one, a number or a flag.
type boundSource struct {
	value     *spec.Value
	exclusive *spec.Value
}

// resolve keeps the stricter of the two bounds, the exclusive one on a tie.
func (b boundSource) resolve(isLower bool) *spec.Bound {
	var out *spec.Bound
	if b.value != nil {
		out = &spec.Bound{Value: b.value.Num}
	}

	switch ex := b.exclusive; {
	case ex == nil:
	case isBool(*ex):
		if out != nil {
			out.Exclusive = ex.Bool
		}
	case out == nil || isBeyond(ex.Num, out.Value, isLower):
		out = &spec.Bound{Value: ex.Num, Exclusive: true}
	}
	return out
}

// isBeyond reports a lower bound a that is at least b, or an upper one that is at most b.
func isBeyond(a, b json.Number, isLower bool) bool {
	x, _ := a.Float64()
	y, _ := b.Float64()
	if isLower {
		return x >= y
	}
	return x <= y
}

// written is a value as a message shows it: a string quoted, a number or a boolean as written.
func written(v spec.Value, n *yaml.Node) string {
	switch v.Kind {
	case spec.KindString:
		return strconv.Quote(v.Str)
	case spec.KindArray:
		return "a list"
	case spec.KindObject:
		return "an object"
	case spec.KindNull:
		return "null"
	case spec.KindNumber, spec.KindBool:
	}
	return n.Value
}

func isNumber(v spec.Value) bool {
	return v.Kind == spec.KindNumber
}

func isBool(v spec.Value) bool {
	return v.Kind == spec.KindBool
}

func isList(v spec.Value) bool {
	return v.Kind == spec.KindArray
}

func isObject(v spec.Value) bool {
	return v.Kind == spec.KindObject
}

// isTypeName accepts all but an object: a scalar that names no type is reported as unknown-type.
func isTypeName(v spec.Value) bool {
	return v.Kind != spec.KindObject
}

func isCount(v spec.Value) bool {
	n, err := v.Num.Int64()
	return isNumber(v) && err == nil && n >= 0
}

func isPositive(v spec.Value) bool {
	f, err := v.Num.Float64()
	return isNumber(v) && err == nil && f > 0
}
