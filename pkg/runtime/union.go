// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"mime/multipart"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Variant describes one member of a union to UnmarshalUnion: the JSON kinds it takes, the
// discriminator values that pick it, and for objects its property names (Known nil takes any key).
// A member that is itself a union lists the objects it can be in Shapes and ranks by the best one.
type Variant struct {
	Name      string
	Kind      Kind
	Values    []string
	IsDefault bool
	Required  []string
	Known     []string
	IsClosed  bool
	Shapes    []Shape
	Into      Setter
}

// Setter sets a variant field from JSON or from a form.
type Setter interface {
	decode(data []byte) error
	fill(form *multipart.Form) error
}

// Shape is one object a variant can be, with property names as Variant has them.
type Shape struct {
	Required []string
	Known    []string
	IsClosed bool
}

// Union describes a union to UnmarshalUnion. Shared are the property names the union holds next
// to its variants, which no variant counts as unknown.
type Union struct {
	IsAnyOf       bool
	Discriminator string
	Shared        []string
	Variants      []Variant
}

// candidate is a variant that can take a value; for objects, score counts the required properties
// present less the unknown keys.
type candidate struct {
	index     int
	score     int
	isMatch   bool
	isPerfect bool
}

// into is the Setter of a variant field.
type into[T any] struct {
	dst *T
}

// Into returns the Setter of the variant field dst, which sets it only when a value decodes.
func Into[T any](dst *T) Setter {
	return into[T]{dst: dst}
}

func (t into[T]) decode(data []byte) error {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*t.dst = v
	return nil
}

func (t into[T]) fill(form *multipart.Form) error {
	var v T
	if err := fillForm(reflect.ValueOf(&v).Elem(), form); err != nil {
		return err
	}
	*t.dst = v
	return nil
}

// UnmarshalUnion decodes data into the variants of u it matches: one for oneOf, every match for
// anyOf. docs/types.md in the mockzilla-codegen repository has the order variants are tried in.
func UnmarshalUnion(data []byte, u Union) error {
	kind := JSONKind(data)
	switch kind {
	case 0:
		return fmt.Errorf("%w: %q is no JSON value", ErrNoVariant, data)
	case KindNull:
		return nil
	default:
	}

	var obj map[string]json.RawMessage
	if kind == KindObject {
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
	}
	return u.setVariants(obj, kind, "a JSON "+kind.String(), func(t Setter) error { return t.decode(data) })
}

// UnmarshalUnionForm reads form into the shared fields, then into the variants it matches.
func UnmarshalUnionForm(form *multipart.Form, fields any, u Union) error {
	if fields != nil {
		if err := fillPointer(form, fields); err != nil {
			return err
		}
	}
	return u.setVariants(formMembers(form), KindObject, "a form", func(t Setter) error { return t.fill(form) })
}

// UnmarshalUnionText decodes raw with a union's UnmarshalJSON, as a number or boolean first.
func UnmarshalUnionText(raw []byte, decode func(data []byte) error) error {
	quoted, _ := json.Marshal(string(raw))
	if !isLiteral(raw) {
		return decode(quoted)
	}

	err := decode(raw)
	if err == nil || decode(quoted) == nil {
		return nil
	}
	return err
}

// setVariants decodes into the variants a value of kind matches; what names the value in errors.
func (u Union) setVariants(obj map[string]json.RawMessage, kind Kind, what string, decode func(Setter) error) error {
	pool := make([]int, len(u.Variants))
	for i := range pool {
		pool[i] = i
	}
	if u.Discriminator != "" && obj != nil {
		picked, rest, err := u.discriminate(obj)
		switch {
		case err != nil:
			return err
		case picked >= 0:
			return decode(u.Variants[picked].Into)
		}
		pool = rest
	}

	cands := u.candidates(pool, kind, obj)
	if u.IsAnyOf {
		return u.decodeAll(cands, what, decode)
	}
	return u.decodeOne(cands, what, decode)
}

// tag fills an empty discriminator value and checks that the value picks a variant of set.
func (u Union) tag(data []byte, set []int) ([]byte, error) {
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(data, &obj) // MarshalUnion wrote an object
	value := discriminatorValue(obj[u.Discriminator])
	first := u.Variants[set[0]]
	if value == "" && len(set) == 1 && len(first.Values) == 1 {
		value = first.Values[0]
		obj[u.Discriminator], _ = json.Marshal(value)
		tag, _ := json.Marshal(map[string]string{u.Discriminator: value})
		data, _ = MergeObjects(data, tag) // both are objects
	}

	picked, rest, err := u.discriminate(obj)
	isSet := func(i int) bool { return slices.Contains(set, i) }
	subject := strconv.Quote(value)
	if value == "" {
		subject = "an empty value"
	}
	var msg string
	switch {
	case picked >= 0 && isSet(picked):
		return data, nil
	case picked >= 0:
		msg = subject + " picks " + u.Variants[picked].Name + ", not " + u.names(set)
	case value == "" && len(first.Values) > 0:
		msg = "must be set, " + first.Name + " takes " + strings.Join(first.Values, " or ")
	case err == nil && slices.ContainsFunc(rest, isSet):
		return data, nil
	default:
		msg = subject + " picks no variant"
	}
	return nil, ValidationError{Field: u.Discriminator, Message: msg}
}

func (u Union) names(set []int) string {
	names := make([]string, len(set))
	for i, v := range set {
		names[i] = u.Variants[v].Name
	}
	return strings.Join(names, " and ")
}

// discriminate returns the variant the discriminator value picks, else -1 and the variants left to
// match by shape: all of them without the property, those without values for an unknown value.
func (u Union) discriminate(obj map[string]json.RawMessage) (int, []int, error) {
	raw, hasValue := obj[u.Discriminator]
	value := discriminatorValue(raw)

	picked := -1
	var all, open []int
	var allowed []string
	for i, v := range u.Variants {
		if hasValue && slices.Contains(v.Values, value) {
			return i, nil, nil
		}
		if v.IsDefault {
			picked = i
		}
		if len(v.Values) == 0 {
			open = append(open, i)
		}
		all = append(all, i)
		allowed = append(allowed, v.Values...)
	}

	switch {
	case picked >= 0:
		return picked, nil, nil
	case !hasValue:
		return -1, all, nil
	case len(open) > 0:
		return -1, open, nil
	}
	return -1, nil, fmt.Errorf("%w %q, want one of %s", ErrUnknownDiscriminator, value, strings.Join(allowed, ", "))
}

func (u Union) candidates(pool []int, kind Kind, obj map[string]json.RawMessage) []candidate {
	var out []candidate
	for _, i := range pool {
		v := u.Variants[i]
		if v.Kind&kind == 0 {
			continue
		}

		c := candidate{isMatch: true}
		if kind == KindObject {
			shapes := v.Shapes
			if len(shapes) == 0 {
				shapes = []Shape{{Required: v.Required, Known: v.Known, IsClosed: v.IsClosed}}
			}
			isFit := false
			for _, sh := range shapes {
				if fc, ok := u.fit(sh, obj); ok && (!isFit || fc.outranks(c)) {
					c, isFit = fc, true
				}
			}
			if !isFit {
				continue
			}
		}
		c.index = i
		out = append(out, c)
	}

	slices.SortStableFunc(out, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(u.width(a), u.width(b)))
	})
	return out
}

// fit scores obj against sh; an unknown key rules out a closed shape.
func (u Union) fit(sh Shape, obj map[string]json.RawMessage) (candidate, bool) {
	absentKeys := 0
	for _, name := range sh.Required {
		if _, found := obj[name]; !found {
			absentKeys++
		}
	}
	unknown := 0
	for key := range obj {
		if sh.Known != nil && !slices.Contains(sh.Known, key) && !slices.Contains(u.Shared, key) {
			unknown++
		}
	}
	if sh.IsClosed && unknown > 0 {
		return candidate{}, false
	}
	return candidate{score: len(sh.Required) - absentKeys - unknown, isMatch: absentKeys == 0, isPerfect: absentKeys == 0 && unknown == 0}, true
}

// width is the number of kinds a candidate takes: an int goes before a float64 for an integer.
func (u Union) width(c candidate) int {
	return bits.OnesCount8(uint8(u.Variants[c.index].Kind))
}

// decodeOne sets the first candidate that decodes. Two perfect object matches with the same score
// are ambiguous.
func (u Union) decodeOne(cands []candidate, what string, decode func(Setter) error) error {
	if len(cands) > 1 && cands[0].isPerfect && cands[1].isPerfect && cands[0].score == cands[1].score {
		return fmt.Errorf("%w: %s and %s", ErrAmbiguous, u.Variants[cands[0].index].Name, u.Variants[cands[1].index].Name)
	}
	return u.decodeFirst(cands, what, decode)
}

// decodeAll sets every matching candidate that decodes, and falls back to the first that decodes
// when none does.
func (u Union) decodeAll(cands []candidate, what string, decode func(Setter) error) error {
	isSet := false
	for _, c := range cands {
		if c.isMatch && decode(u.Variants[c.index].Into) == nil {
			isSet = true
		}
	}
	if isSet {
		return nil
	}
	return u.decodeFirst(cands, what, decode)
}

func (u Union) decodeFirst(cands []candidate, what string, decode func(Setter) error) error {
	var errs []error
	for _, c := range cands {
		v := u.Variants[c.index]
		err := decode(v.Into)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", v.Name, err))
	}
	if len(errs) == 0 {
		return fmt.Errorf("%w for %s", ErrNoVariant, what)
	}
	return fmt.Errorf("%w for %s: %w", ErrNoVariant, what, errors.Join(errs...))
}

func (c candidate) outranks(other candidate) bool {
	return c.score > other.score || c.score == other.score && c.isPerfect && !other.isPerfect
}

func discriminatorValue(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(bytes.TrimSpace(raw))
}

// isLiteral reports raw written as a JSON number or boolean, with no space around it.
func isLiteral(raw []byte) bool {
	switch JSONKind(raw) {
	case KindBool, KindInteger, KindNumber:
		return json.Valid(raw) && len(bytes.TrimSpace(raw)) == len(raw)
	default:
	}
	return false
}

// formMembers are the names of form as object members, each text as a JSON string.
func formMembers(form *multipart.Form) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for key := range form.File {
		out[formName(key)] = json.RawMessage("{}")
	}
	for key, values := range form.Value {
		name := formName(key)
		switch {
		case name == key && len(values) > 0:
			out[name], _ = json.Marshal(values[0])
		case out[name] == nil:
			out[name] = json.RawMessage("{}")
		}
	}
	delete(out, "")
	return out
}
