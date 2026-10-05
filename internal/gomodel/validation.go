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

	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// ruleGroup is the group of rules that apply to a Go value.
type ruleGroup int

const (
	groupOther ruleGroup = iota
	groupString
	groupBytes
	groupNumber
	groupBool
	groupSlice
	groupMap
)

// checkedFormats are the string formats runtime.Format checks.
var checkedFormats = []string{"uuid", "uri", "uri-reference", "ipv4", "ipv6", "hostname", "date", "date-time", "email"}

// keywordSet holds the keywords that constrain a value where it is used.
type keywordSet struct {
	jointChecks
	limits     spec.Limits
	format     string
	constant   *spec.Value
	enumSchema *spec.Schema
}

// schemaChain follows a schema where it is used through its plain refs.
type schemaChain struct {
	flat  *flattener
	decls map[*spec.Schema]*Decl
}

// validator plans the Validate methods once every type is settled. A declaration gets them when
// it checks something, itself or through the types it holds.
type validator struct {
	schemaChain
	opts       Options
	patternSet *patternSet
}

func newValidator(opts Options, flat *flattener, decls map[*spec.Schema]*Decl, patterns *patternSet) *validator {
	return &validator{schemaChain: schemaChain{flat: flat, decls: decls}, opts: opts, patternSet: patterns}
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
		return &Validation{Checks: slices.Concat(v.ownChecks(d), v.structChecks(d))}
	case KindUnion:
		checks := slices.Concat(v.ownChecks(d), v.structChecks(d))
		for _, vr := range d.Union.Variants {
			c := v.check(d, vr.schema, vr.FieldType, d.Name+vr.Name)
			c.Field = vr.Name
			checks = appendCheck(checks, c)
		}
		return &Validation{Count: unionCount(d.Union), IsDiscriminated: d.Union.Discriminator != "", Checks: checks}
	default:
		return &Validation{Checks: appendCheck(nil, v.check(d, d.schema, d.Target, d.Name))}
	}
}

// ownChecks compare a struct or union with its own enum; a type list leaves it to its variants.
func (v *validator) ownChecks(d *Decl) []*Check {
	if d.Union != nil && d.Union.isTypeList {
		return nil
	}
	return appendCheck(nil, &Check{Rules: enumRules(v.keywords(d.schema).enumSchema)})
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
	t = Elem(t)

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
		c.Keys = v.keyRules(d, s, name+"Key")
	}
	return c
}

// keywords gathers the keywords that constrain a value where s is used: its own, those of
// docs-only allOf members, and those of the aliases its refs lead to. A declared type checks its
// own keywords in its Validate.
func (v *validator) keywords(s *spec.Schema) *keywordSet {
	out := &keywordSet{}
	for _, x := range v.chain(s) {
		v.addKeywords(out, x)
	}
	return out
}

func (v *validator) addKeywords(out *keywordSet, x *spec.Schema) {
	f := x
	if m := v.flat.merged(x); m != nil {
		f = m.schema
		out.join(m.joint)
	}
	for _, part := range append([]*spec.Schema{f}, siblings(x)[1:]...) {
		mergeLimits(&out.limits, part.Limits)
		out.add(part)
		out.format = cmp.Or(out.format, part.Format)
		out.constant = cmp.Or(out.constant, part.Const)
		if out.enumSchema == nil && len(part.Enum) > 0 {
			out.enumSchema = part
		}
	}
}

// keyRules are what propertyNames checks of each key; a key is a string, so every ref is followed.
func (v *validator) keyRules(d *Decl, s *spec.Schema, name string) []Rule {
	names := v.propertyNamesOf(s)
	if names == nil {
		return nil
	}

	kw := &keywordSet{}
	for x, seen := names, map[*spec.Schema]bool{}; x != nil && !seen[x]; {
		seen[x] = true
		v.addKeywords(kw, x)
		r := refOf(x)
		if r == nil {
			break
		}
		x = r.Target
	}

	out := v.rules(d, kw, stringType, name)
	var values []spec.Value
	for _, e := range v.flat.flatten(target(names)).Enum {
		if e.Kind == spec.KindString {
			values = append(values, e)
		}
	}
	if len(values) > 0 {
		out = append(out, Rule{Kind: RuleEnum, Values: values})
	}
	return out
}

// rules are the keyword checks that fit a value of type t.
func (v *validator) rules(d *Decl, kw *keywordSet, t Type, name string) []Rule {
	var out []Rule
	lim := kw.limits
	switch g := groupOf(t); g {
	case groupString, groupBytes:
		out = appendCount(out, RuleMinLength, lim.MinLength)
		out = appendCount(out, RuleMaxLength, lim.MaxLength)
		out = append(out, v.patterns(d, kw, name)...)
		if g == groupBytes {
			for i := range out {
				out[i].IsBase64 = true
			}
			break
		}
		if f := strings.ToLower(kw.format); slices.Contains(checkedFormats, f) && unalias(t) != emailType {
			out = append(out, Rule{Kind: RuleFormat, Format: f})
		}
		out = appendConst(out, kw.constant, kw.constant != nil && kw.constant.Kind == spec.KindString)
	case groupNumber:
		out = appendBound(out, RuleMinimum, lim.Minimum)
		out = appendBound(out, RuleMaximum, lim.Maximum)
		for _, m := range kw.multiples {
			out = append(out, Rule{Kind: RuleMultipleOf, Number: m.String()})
		}
		isNumber := kw.constant != nil && kw.constant.Kind == spec.KindNumber
		out = appendConst(out, kw.constant, isNumber && (!isInteger(t) || isIntegral(kw.constant.Num)))
	case groupBool:
		out = appendConst(out, kw.constant, kw.constant != nil && kw.constant.Kind == spec.KindBool)
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
	if isJSONValue(t) {
		out = append(out, enumRules(kw.enumSchema)...)
	}
	return out
}

// patterns are the rules of the patterns of kw; a pattern RE2 cannot compile has none.
func (v *validator) patterns(d *Decl, kw *keywordSet, name string) []Rule {
	var out []Rule
	for _, s := range kw.patterns {
		if p := v.patternSet.add(d.Part, s.Pattern, s.Origin, name); p != nil {
			out = append(out, Rule{Kind: RulePattern, Pattern: p})
		}
	}
	return out
}

// chain is s and the schemas its plain refs lead to, stopping before a declared type that is no
// alias.
func (c schemaChain) chain(s *spec.Schema) []*spec.Schema {
	var out []*spec.Schema
	for s != nil && !slices.Contains(out, s) {
		if d := c.decls[s]; d != nil && d.Kind != KindAlias && len(out) > 0 {
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

func (c schemaChain) propertyNamesOf(s *spec.Schema) *spec.Schema {
	for _, x := range c.chain(s) {
		if f := c.flat.flatten(x); f.PropertyNames != nil {
			return f.PropertyNames
		}
	}
	return nil
}

func (c schemaChain) itemsOf(s *spec.Schema) *spec.Schema {
	for _, x := range c.chain(s) {
		if f := c.flat.flatten(x); f.Items != nil {
			return f.Items
		}
	}
	return nil
}

func (c schemaChain) valuesOf(s *spec.Schema) *spec.Schema {
	for _, x := range c.chain(s) {
		if f := c.flat.flatten(x); f.AdditionalProperties.Mode == spec.AdditionalSchema {
			return f.AdditionalProperties.Schema
		}
	}
	return nil
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
		var live []*Check
		for _, c := range d.Validation.Checks {
			live = appendCheck(live, dropUnchecked(c, checked))
		}
		d.Validation.Checks = live
	}
}

func isChecked(d *Decl, checked map[*Decl]bool) bool {
	isLive := func(c *Check) bool { return !isEmpty(dropUnchecked(c, checked)) }
	return d.Enum != nil && len(d.Enum.Values) > 0 || d.Validation.Count != "" || d.Validation.IsDiscriminated ||
		slices.ContainsFunc(d.Validation.Checks, isLive)
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
		if t.Elem == byteType {
			return groupBytes
		}
		return groupSlice
	case Map:
		return groupMap
	case DeclRef:
		if t.Decl.Kind == KindDefined {
			return groupOf(t.Decl.Target)
		}
	}
	return groupOther
}

// isJSONValue reports a list, map or any, whose enum is checked where it is used.
func isJSONValue(t Type) bool {
	switch t := unalias(t).(type) {
	case Slice:
		return t.Elem != byteType
	case Map:
		return true
	case Builtin:
		return t == anyType
	}
	return false
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
	return len(c.Rules) == 0 && !c.IsNested && c.Items == nil && c.Values == nil && len(c.Keys) == 0 && !c.IsRequired
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

// enumRules compare a value as JSON with the values of the enum of s that fit s, but null.
func enumRules(s *spec.Schema) []Rule {
	if s == nil {
		return nil
	}
	var values []spec.Value
	for _, e := range s.Enum {
		if e.Kind != spec.KindNull && jsonschema.Misfit(e, s) == "" {
			values = append(values, e)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return []Rule{{Kind: RuleEnumJSON, Values: values}}
}

func appendConst(out []Rule, c *spec.Value, isFit bool) []Rule {
	if !isFit {
		return out
	}
	return append(out, Rule{Kind: RuleConst, Const: *c})
}
