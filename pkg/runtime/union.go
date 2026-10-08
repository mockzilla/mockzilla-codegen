// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Decoding oneOf and anyOf unions: which variants a value matches.

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

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
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
// to its variants, which no variant counts as unknown. Also are more unions the value is at once.
type Union struct {
	IsAnyOf       bool
	Discriminator string
	Shared        []string
	Variants      []Variant
	Also          []Union
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
	if err := fillForm(reflect.ValueOf(&v).Elem(), form, nil); err != nil {
		return err
	}
	*t.dst = v
	return nil
}

// UnmarshalUnion decodes data into the variants of u it matches: one for oneOf, every match for
// anyOf. docs/types.md in the mockzilla-codegen repository has the order variants are tried in.
func UnmarshalUnion(data []byte, u Union) error {
	kind := jsonKind(data)
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
	return u.setAll(obj, kind, "a JSON "+kind.String(), func(t Setter) error { return t.decode(data) })
}

// UnmarshalUnionForm reads form into the shared fields, then into the variants it matches.
func UnmarshalUnionForm(form *multipart.Form, fields any, u Union) error {
	if fields != nil {
		if err := fillPointer(form, fields); err != nil {
			return err
		}
	}
	return u.setAll(formMembers(form), KindObject, "a form", func(t Setter) error { return t.fill(form) })
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

// groups are u and the unions in its Also, each with the shared names of u.
func (u Union) groups() []Union {
	out := []Union{u}
	for _, g := range u.Also {
		g.Shared = u.Shared
		out = append(out, g)
	}
	return out
}

// setAll decodes into the variants of each group a value of kind matches.
func (u Union) setAll(obj map[string]json.RawMessage, kind Kind, what string, decode func(Setter) error) error {
	for _, g := range u.groups() {
		if err := g.setVariants(obj, kind, what, decode); err != nil {
			return err
		}
	}
	return nil
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

	cands, err := u.candidates(pool, kind, obj, what)
	switch {
	case err != nil:
		return err
	case u.IsAnyOf:
		return u.decodeAll(cands, what, decode)
	case kind == KindObject && len(cands) > 1 && cands[0].score == cands[1].score:
		return fmt.Errorf("%w: %s and %s", ErrAmbiguous, u.Variants[cands[0].index].Name, u.Variants[cands[1].index].Name)
	}
	return u.decodeFirst(cands, what, decode)
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
		data, _ = mergeObjects(data, tag) // both are objects
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
	return nil, validation.Error{Field: u.Discriminator, Message: msg, Rule: validation.RuleDiscriminator}
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

// candidates are the variants a value of kind matches, best first.
func (u Union) candidates(pool []int, kind Kind, obj map[string]json.RawMessage, what string) ([]candidate, error) {
	var out []candidate
	var needs []string
	for _, i := range pool {
		v := u.Variants[i]
		if v.Kind&kind == 0 {
			continue
		}

		c := candidate{isMatch: true}
		if kind == KindObject {
			var lacks []string
			c, lacks = u.bestShape(v, obj)
			if !c.isMatch {
				if lacks != nil {
					needs = append(needs, v.Name+" needs "+strings.Join(lacks, " or "))
				}
				continue
			}
		}
		c.index = i
		out = append(out, c)
	}
	if len(out) == 0 && len(needs) > 0 {
		return nil, fmt.Errorf("%w for %s: %s", ErrNoVariant, what, strings.Join(needs, ", "))
	}

	slices.SortStableFunc(out, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(u.width(a), u.width(b)))
	})
	return out, nil
}

// bestShape is the best shape of v that obj matches, else what obj lacks for each shape.
func (u Union) bestShape(v Variant, obj map[string]json.RawMessage) (best candidate, lacks []string) {
	shapes := v.Shapes
	if len(shapes) == 0 {
		shapes = []Shape{{Required: v.Required, Known: v.Known, IsClosed: v.IsClosed}}
	}
	for _, sh := range shapes {
		c, unmet, ok := u.fit(sh, obj)
		switch {
		case !ok:
		case !c.isMatch:
			lacks = append(lacks, strings.Join(unmet, " and "))
		case !best.isMatch || c.outranks(best):
			best = c
		}
	}
	return best, lacks
}

// fit scores obj against sh and lists its unmet required keys; unknown keys rule out a closed sh.
func (u Union) fit(sh Shape, obj map[string]json.RawMessage) (candidate, []string, bool) {
	var unmet []string
	for _, name := range sh.Required {
		if _, found := obj[name]; !found {
			unmet = append(unmet, name)
		}
	}
	unknown := 0
	for key := range obj {
		if sh.Known != nil && !slices.Contains(sh.Known, key) && !slices.Contains(u.Shared, key) {
			unknown++
		}
	}
	if sh.IsClosed && unknown > 0 {
		return candidate{}, nil, false
	}
	isMatch := len(unmet) == 0
	return candidate{score: len(sh.Required) - len(unmet) - unknown, isMatch: isMatch, isPerfect: isMatch && unknown == 0}, unmet, true
}

// width is the number of kinds a candidate takes: an int goes before a float64 for an integer.
func (u Union) width(c candidate) int {
	return bits.OnesCount8(uint8(u.Variants[c.index].Kind))
}

// decodeAll sets every candidate that decodes.
func (u Union) decodeAll(cands []candidate, what string, decode func(Setter) error) error {
	var errs []error
	for _, c := range cands {
		v := u.Variants[c.index]
		if err := decode(v.Into); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", v.Name, err))
		}
	}
	if len(errs) < len(cands) {
		return nil
	}
	return noVariant(what, errs)
}

// decodeFirst sets the first candidate that decodes.
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
	return noVariant(what, errs)
}

func (c candidate) outranks(other candidate) bool {
	return c.score > other.score || c.score == other.score && c.isPerfect && !other.isPerfect
}

// noVariant is the error of a value no candidate decodes, errs being why each one failed.
func noVariant(what string, errs []error) error {
	if len(errs) == 0 {
		return fmt.Errorf("%w for %s", ErrNoVariant, what)
	}
	return fmt.Errorf("%w for %s: %w", ErrNoVariant, what, errors.Join(errs...))
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
	switch jsonKind(raw) {
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
