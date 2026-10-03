// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The Validate methods: which declarations check something, and the checks each value needs.

package gomodel

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// ruleGroup is the group of rules that apply to a Go value.
type ruleGroup int

const (
	groupOther ruleGroup = iota
	groupString
	groupNumber
	groupBool
	groupSlice
	groupMap
)

// checkedFormats are the string formats runtime.Format checks.
var checkedFormats = []string{"uuid", "uri", "uri-reference", "ipv4", "ipv6", "hostname", "date", "date-time", "email"}

// validator plans the Validate methods once every type is settled. A declaration gets them when
// it checks something, itself or through the types it holds.
type validator struct {
	opts     Options
	flat     *flattener
	decls    map[*spec.Schema]*Decl
	patterns *patternSet
}

func newValidator(opts Options, flat *flattener, decls map[*spec.Schema]*Decl, patterns *patternSet) *validator {
	return &validator{opts: opts, flat: flat, decls: decls, patterns: patterns}
}

// plan sets the Validation of every declaration.
func (v *validator) plan(decls []*Decl) {
	for _, d := range decls {
		d.Validation = v.validation(d)
	}
	keepChecked(decls)
	if v.opts.ValidateResponse {
		markResponse(decls)
	}
}

func (v *validator) validation(d *Decl) *Validation {
	switch d.Kind {
	case KindAlias:
		return nil
	case KindEnum:
		return &Validation{}
	case KindStruct:
		return &Validation{Checks: v.structChecks(d)}
	case KindUnion:
		checks := v.structChecks(d)
		for _, vr := range d.Union.Variants {
			c := v.check(d, vr.schema, vr.FieldType, d.Name+vr.Name)
			c.Field = vr.Name
			checks = appendCheck(checks, c)
		}
		return &Validation{Count: unionCount(d.Union), Checks: checks}
	default:
		return &Validation{Checks: appendCheck(nil, v.check(d, d.schema, d.Target, d.Name))}
	}
}

// structChecks checks each field, then the values of additional properties.
func (v *validator) structChecks(d *Decl) []*Check {
	var out []*Check
	for _, f := range d.Struct.Fields {
		c := v.check(d, f.schema, f.Type, d.Name+f.Name)
		c.Field, c.Path = f.Name, f.JSONName
		c.IsRequired = f.Required && !f.Nullable && nilable(f.Type)
		switch {
		case f.ReadOnly:
			c.Side = SideResponse
		case f.WriteOnly:
			c.Side = SideRequest
		}
		out = appendCheck(out, c)
	}

	if ap := d.Struct.AdditionalProperties; ap != nil {
		c := v.check(d, d.schema, ap.Type, d.Name)
		c.Field, c.Rules = ap.Name, nil
		out = appendCheck(out, c)
	}
	return out
}

// check is what a value of type t, described by s, needs. A nil value, which means absent, is not
// checked.
func (v *validator) check(d *Decl, s *spec.Schema, t Type, name string) *Check {
	c := &Check{IsPointer: isPointer(t), IsGuarded: nilable(t)}
	t = elem(t)

	kw := v.keywords(s)
	c.Rules = v.rules(d, kw, t, name)
	c.Nested, c.IsNested = validated(t)
	switch u := t.(type) {
	case Slice:
		if u.Elem != byteType {
			c.Items = nonEmpty(v.check(d, v.itemsOf(s), u.Elem, name+"Item"))
		}
	case Map:
		c.Values = nonEmpty(v.check(d, v.valuesOf(s), u.Elem, name+"Value"))
	}
	return c
}

// keywords gathers the keywords that constrain a value where s is used: its own, those of
// docs-only allOf members, and those of the aliases its refs lead to. A declared type checks its
// own keywords in its Validate.
func (v *validator) keywords(s *spec.Schema) *spec.Schema {
	out := &spec.Schema{}
	for _, x := range v.chain(s) {
		for _, part := range append([]*spec.Schema{v.flat.flatten(x)}, siblings(x)[1:]...) {
			mergeLimits(&out.Limits, part.Limits)
			if out.Pattern == "" && part.Pattern != "" {
				out.Pattern, out.Origin = part.Pattern, part.Origin
			}
			out.Format = cmp.Or(out.Format, part.Format)
			out.Const = cmp.Or(out.Const, part.Const)
		}
	}
	return out
}

// chain is s and the schemas its plain refs lead to, stopping before a declared type that is no
// alias.
func (v *validator) chain(s *spec.Schema) []*spec.Schema {
	var out []*spec.Schema
	for s != nil && !slices.Contains(out, s) {
		if d := v.decls[s]; d != nil && d.Kind != KindAlias && len(out) > 0 {
			break
		}
		out = append(out, s)
		r := refOf(s)
		if r == nil {
			break
		}
		s = r.Target
	}
	return out
}

func (v *validator) itemsOf(s *spec.Schema) *spec.Schema {
	for _, x := range v.chain(s) {
		if f := v.flat.flatten(x); f.Items != nil {
			return f.Items
		}
	}
	return nil
}

func (v *validator) valuesOf(s *spec.Schema) *spec.Schema {
	for _, x := range v.chain(s) {
		if f := v.flat.flatten(x); f.AdditionalProperties.Mode == spec.AdditionalSchema {
			return f.AdditionalProperties.Schema
		}
	}
	return nil
}

// rules are the keyword checks that fit a value of type t.
func (v *validator) rules(d *Decl, kw *spec.Schema, t Type, name string) []Rule {
	var out []Rule
	lim := kw.Limits
	switch groupOf(t) {
	case groupString:
		out = appendCount(out, RuleMinLength, lim.MinLength)
		out = appendCount(out, RuleMaxLength, lim.MaxLength)
		if p := v.pattern(d, kw, name); p != nil {
			out = append(out, Rule{Kind: RulePattern, Pattern: p})
		}
		if f := strings.ToLower(kw.Format); slices.Contains(checkedFormats, f) && unalias(t) != emailType {
			out = append(out, Rule{Kind: RuleFormat, Format: f})
		}
		out = appendConst(out, kw.Const, kw.Const != nil && kw.Const.Kind == spec.KindString)
	case groupNumber:
		out = appendBound(out, RuleMinimum, lim.Minimum)
		out = appendBound(out, RuleMaximum, lim.Maximum)
		if m := lim.MultipleOf; m != nil {
			out = append(out, Rule{Kind: RuleMultipleOf, Number: m.String()})
		}
		isNumber := kw.Const != nil && kw.Const.Kind == spec.KindNumber
		out = appendConst(out, kw.Const, isNumber && (!isInteger(t) || isIntegral(kw.Const.Num)))
	case groupBool:
		out = appendConst(out, kw.Const, kw.Const != nil && kw.Const.Kind == spec.KindBool)
	case groupSlice:
		out = appendCount(out, RuleMinItems, lim.MinItems)
		out = appendCount(out, RuleMaxItems, lim.MaxItems)
		if lim.UniqueItems {
			kind := RuleUniqueJSON
			if s, ok := unalias(t).(Slice); ok && isComparable(s.Elem) {
				kind = RuleUnique
			}
			out = append(out, Rule{Kind: kind})
		}
	case groupMap:
		out = appendCount(out, RuleMinProperties, lim.MinProperties)
		out = appendCount(out, RuleMaxProperties, lim.MaxProperties)
	case groupOther:
	}
	return out
}

// pattern returns the variable that holds the pattern of kw in the part of d, or nil when there is
// no pattern or RE2 cannot compile it.
func (v *validator) pattern(d *Decl, kw *spec.Schema, name string) *Pattern {
	if kw.Pattern == "" {
		return nil
	}
	return v.patterns.add(d.Part, kw.Pattern, kw.Origin, name)
}

// keepChecked drops the Validation of declarations that check nothing, and the nested calls to
// them. A declaration that only holds such types checks nothing either, loops of them included.
func keepChecked(decls []*Decl) {
	checked := map[*Decl]bool{}
	for isChanged := true; isChanged; {
		isChanged = false
		for _, d := range decls {
			if d.Validation != nil && !checked[d] && isChecked(d, checked) {
				checked[d] = true
				isChanged = true
			}
		}
	}

	for _, d := range decls {
		if !checked[d] {
			d.Validation = nil
			continue
		}
		var kept []*Check
		for _, c := range d.Validation.Checks {
			kept = appendCheck(kept, dropUnchecked(c, checked))
		}
		d.Validation.Checks = kept
	}
}

func isChecked(d *Decl, checked map[*Decl]bool) bool {
	isLive := func(c *Check) bool { return !isEmpty(dropUnchecked(c, checked)) }
	return d.Enum != nil && len(d.Enum.Values) > 0 || d.Validation.Count != "" || slices.ContainsFunc(d.Validation.Checks, isLive)
}

// dropUnchecked returns c without the nested calls to declarations that check nothing.
func dropUnchecked(c *Check, checked map[*Decl]bool) *Check {
	if c == nil {
		return nil
	}
	out := *c
	if out.Nested != nil && !checked[out.Nested] {
		out.Nested, out.IsNested = nil, false
	}
	out.Items = nonEmpty(dropUnchecked(c.Items, checked))
	out.Values = nonEmpty(dropUnchecked(c.Values, checked))
	return &out
}

// markResponse gives ValidateResponse to every declaration whose checks differ by side, and then to
// every declaration that checks one of those, until nothing changes.
func markResponse(decls []*Decl) {
	for isChanged := true; isChanged; {
		isChanged = false
		for _, d := range decls {
			if d.Validation != nil && !d.Validation.HasResponse && slices.ContainsFunc(d.Validation.Checks, differs) {
				d.Validation.HasResponse = true
				isChanged = true
			}
		}
	}
}

func differs(c *Check) bool {
	return c != nil && (c.Side != SideBoth || c.Nested != nil && c.Nested.Validation.HasResponse || differs(c.Items) || differs(c.Values))
}

// unionCount is the runtime check of how many variants a union has set.
func unionCount(u *Union) string {
	switch {
	case u.IsAnyOf && u.IsNullable:
		return ""
	case u.IsAnyOf:
		return "AtLeastOne"
	case u.IsNullable:
		return "AtMostOne"
	}
	return "ExactlyOne"
}

// validated returns the declaration whose Validate a value of type t has, through aliases. A
// runtime.Email has one of its own and no declaration.
func validated(t Type) (*Decl, bool) {
	t = unalias(t)
	if r, ok := t.(DeclRef); ok {
		return r.Decl, true
	}
	return nil, t == emailType
}

// unalias follows aliases to the type they stand for.
func unalias(t Type) Type {
	for {
		r, ok := t.(DeclRef)
		if !ok || r.Decl.Kind != KindAlias {
			return t
		}
		t = r.Decl.Target
	}
}

func groupOf(t Type) ruleGroup {
	switch t := unalias(t).(type) {
	case Builtin:
		switch builtinKinds(t.Name) {
		case JSONString:
			return groupString
		case JSONBool:
			return groupBool
		case JSONInteger, JSONInteger | JSONNumber:
			return groupNumber
		default:
		}
	case Qualified:
		if t == emailType {
			return groupString
		}
	case Slice:
		if t.Elem != byteType {
			return groupSlice
		}
	case Map:
		return groupMap
	case DeclRef:
		if t.Decl.Kind == KindDefined {
			return groupOf(t.Decl.Target)
		}
	}
	return groupOther
}

func isInteger(t Type) bool {
	b, ok := unalias(t).(Builtin)
	return ok && builtinKinds(b.Name) == JSONInteger
}

// isComparable reports item types Go compares by value: builtins and enums.
func isComparable(t Type) bool {
	switch t := unalias(t).(type) {
	case Builtin:
		return t != anyType
	case DeclRef:
		return t.Decl.Kind == KindEnum
	}
	return false
}

func isPointer(t Type) bool {
	_, ok := t.(Pointer)
	return ok
}

func isEmpty(c *Check) bool {
	return len(c.Rules) == 0 && !c.IsNested && c.Items == nil && c.Values == nil && !c.IsRequired
}

func nonEmpty(c *Check) *Check {
	if c == nil || isEmpty(c) {
		return nil
	}
	return c
}

func appendCheck(out []*Check, c *Check) []*Check {
	if isEmpty(c) {
		return out
	}
	return append(out, c)
}

func appendCount(out []Rule, kind RuleKind, n *int64) []Rule {
	if n == nil {
		return out
	}
	return append(out, Rule{Kind: kind, Number: strconv.FormatInt(*n, 10)})
}

func appendBound(out []Rule, kind RuleKind, b *spec.Bound) []Rule {
	if b == nil {
		return out
	}
	return append(out, Rule{Kind: kind, Number: b.Value.String(), IsExclusive: b.Exclusive})
}

func appendConst(out []Rule, c *spec.Value, isFit bool) []Rule {
	if !isFit {
		return out
	}
	return append(out, Rule{Kind: RuleConst, Const: *c})
}
