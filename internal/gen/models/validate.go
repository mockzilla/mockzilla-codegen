// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of validate.tmpl, check.tmpl and error.tmpl: the Validate methods of a declaration,
// each check a runtime call, and the Error method of an error type.

package models

import (
	"slices"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
)

// ruleFuncs are the checks of the validation package, by rule kind.
var ruleFuncs = map[gomodel.RuleKind]string{
	gomodel.RuleMinLength:     "MinLength",
	gomodel.RuleMaxLength:     "MaxLength",
	gomodel.RulePattern:       "Pattern",
	gomodel.RuleFormat:        "Format",
	gomodel.RuleMinimum:       "Minimum",
	gomodel.RuleMaximum:       "Maximum",
	gomodel.RuleMultipleOf:    "MultipleOf",
	gomodel.RuleMinItems:      "MinItems",
	gomodel.RuleMaxItems:      "MaxItems",
	gomodel.RuleUnique:        "Unique",
	gomodel.RuleUniqueJSON:    "UniqueJSON",
	gomodel.RuleMinProperties: "MinProperties",
	gomodel.RuleMaxProperties: "MaxProperties",
	gomodel.RuleConst:         "Const",
	gomodel.RuleEnum:          "Enum",
	gomodel.RuleEnumJSON:      "EnumJSON",
}

// ValidateView is what the Validate methods of a declaration need. Enum lists the constants an enum
// takes; Response is set when ValidateResponse checks something else than Validate.
type ValidateView struct {
	Receiver   string
	Validation string
	Enum       []string
	Request    MethodView
	Response   *MethodView
}

// MethodView is the body of one Validate method; all but Validation and Checks are union checks.
type MethodView struct {
	Validation    string
	Counts        []string
	Discriminator string
	Checks        []CheckView
	VariantChecks []CheckView
	AnyValid      [][]VariantCheckView
}

// VariantCheckView is one variant of an anyOf; Check is nil when it checks nothing.
type VariantCheckView struct {
	IsSet string
	Check *CheckView
}

// CheckView checks one value. Every text is a Go expression: Calls return an error that is added
// to Errs under Path, Deref is Value behind its pointer. Guard is what the calls run under,
// NullCheck the call that rejects a null Nullable.
type CheckView struct {
	Errs       string
	Path       string
	Value      string
	Deref      string
	Guard      string
	IsRequired bool
	NullCheck  string
	Calls      []string
	Items      *LoopView
	Values     *LoopView
}

// LoopView checks each item of a slice, or each key and value of a map in key order. Range is what
// the loop ranges over, Index and Item its variables, Map the map; Key or Check is nil when unused.
type LoopView struct {
	Index string
	Item  string
	Range string
	Map   string
	Key   *CheckView
	Check *CheckView
}

// ErrorView is what the Error method and the constructor of an error type need.
type ErrorView struct {
	Receiver       string
	Runtime        string
	Path           string
	PathLiteral    string
	Fallback       string
	HasConstructor bool
}

// PatternView is one package-level regular expression.
type PatternView struct {
	Name   string
	Regexp string
	Source string
}

// methodSide is what one Validate method leaves out, and whether it is ValidateResponse.
type methodSide struct {
	skip       gomodel.Side
	isResponse bool
}

// checkAt is where a check is written: its value and error path as Go expressions, how deep in
// loops it sits, and the variable that collects its errors.
type checkAt struct {
	value string
	path  string
	depth int
	errs  string
}

func validateView(d *gomodel.Decl, s *gocode.Scope) *ValidateView {
	v := d.Validation
	out := &ValidateView{Receiver: receiver(d.Name)}
	if d.Enum != nil {
		out.Validation = s.Import(gomodel.Import{Path: gomodel.ValidationPath})
		for _, ev := range d.Enum.Values {
			out.Enum = append(out.Enum, ev.Name)
		}
	}

	out.Request = methodView(d, s, methodSide{skip: gomodel.SideResponse})
	if v.HasResponse {
		response := methodView(d, s, methodSide{skip: gomodel.SideRequest, isResponse: true})
		out.Response = &response
	}
	return out
}

func methodView(d *gomodel.Decl, s *gocode.Scope, side methodSide) MethodView {
	v := d.Validation
	r := receiver(d.Name)
	isChecked := len(v.Counts) > 0 || v.IsDiscriminated ||
		slices.ContainsFunc(v.Checks, func(c *gomodel.Check) bool { return c.Side != side.skip })
	if !isChecked {
		return MethodView{}
	}

	vd := s.Import(gomodel.Import{Path: gomodel.ValidationPath})
	m := MethodView{Validation: vd}
	for _, c := range v.Counts {
		args := make([]string, len(c.Variants))
		for i, vr := range c.Variants {
			args[i] = gocode.NotNil(gocode.Selector(r, vr.Name))
		}
		m.Counts = append(m.Counts, gocode.Call(gocode.Selector(vd, c.Func), args...))
	}

	if v.IsDiscriminated {
		rt := s.Import(gomodel.Import{Path: gomodel.RuntimePath})
		m.Discriminator = gocode.Call(gocode.Selector(rt, "DiscriminatorError"), gocode.Call(gocode.Selector(r, "MarshalJSON")))
	}

	isAnyOfOnly := anyOfOnly(d)
	byVariant := map[string]*CheckView{}
	for _, c := range v.Checks {
		if c.Side == side.skip {
			continue
		}
		at := checkAt{value: r, path: gocode.Quote(c.Path), depth: 1, errs: "errs"}
		if c.Field != "" {
			at.value = gocode.Selector(r, c.Field)
		}
		if c.IsVariant && isAnyOfOnly[c.Field] {
			at.errs = "errs" + c.Field
			cv := checkView(c, at, vd, side)
			byVariant[c.Field] = &cv
			m.VariantChecks = append(m.VariantChecks, cv)
			continue
		}
		m.Checks = append(m.Checks, checkView(c, at, vd, side))
	}

	if d.Union == nil {
		return m
	}
	for _, g := range d.Union.Groups {
		if !g.IsAnyOf || !slices.ContainsFunc(g.Variants, func(vr *gomodel.Variant) bool { return byVariant[vr.Name] != nil }) {
			continue
		}
		list := make([]VariantCheckView, len(g.Variants))
		for i, vr := range g.Variants {
			list[i] = VariantCheckView{IsSet: gocode.NotNil(gocode.Selector(r, vr.Name)), Check: byVariant[vr.Name]}
		}
		m.AnyValid = append(m.AnyValid, list)
	}
	return m
}

// anyOfOnly marks the variants only anyOf groups list; a oneOf variant must pass its checks.
func anyOfOnly(d *gomodel.Decl) map[string]bool {
	out := map[string]bool{}
	if d.Union == nil {
		return out
	}
	for _, g := range d.Union.Groups {
		for _, vr := range g.Variants {
			if _, ok := out[vr.Name]; !ok || !g.IsAnyOf {
				out[vr.Name] = g.IsAnyOf
			}
		}
	}
	return out
}

// checkView writes check c of the value at. Loop variables get the depth as a suffix below the
// first level.
func checkView(c *gomodel.Check, at checkAt, vd string, side methodSide) CheckView {
	value, path := at.value, at.path
	suffix := ""
	if at.depth > 1 {
		suffix = strconv.Itoa(at.depth)
	}
	deref, owner, guard := value, value, gocode.NotNil(value)
	switch {
	case c.IsWrapped:
		deref, owner = "value"+suffix, "value"+suffix
		guard = gocode.Get(value, deref, "ok"+suffix)
	case c.IsPointer:
		deref = gocode.Deref(value)
	}

	cv := CheckView{Errs: at.errs, Path: path, Value: value, Deref: deref, IsRequired: c.IsRequired}
	if c.IsNullRejected {
		cv.NullCheck = gocode.Call(gocode.Selector(vd, "NotNull"), value)
	}
	for _, r := range c.Rules {
		cv.Calls = append(cv.Calls, ruleCall(r, vd, deref))
	}
	if c.IsNested {
		method := "Validate"
		if side.isResponse && c.Nested != nil && c.Nested.Validation.HasResponse {
			method = "ValidateResponse"
		}
		cv.Calls = append(cv.Calls, gocode.Call(gocode.Selector(owner, method)))
	}
	// Ranging over a nil slice or map is fine; only calls and the value of a Nullable need it.
	isLooped := c.Items != nil || c.Values != nil || len(c.Keys) > 0
	if c.IsGuarded && (len(cv.Calls) > 0 || c.IsWrapped && isLooped) {
		cv.Guard = guard
	}

	if c.Items != nil {
		index, item := "idx"+suffix, "item"+suffix
		cv.Items = &LoopView{
			Index: index,
			Item:  item,
			Range: deref,
			Check: new(checkView(c.Items, checkAt{value: item, path: gocode.Call(gocode.Selector(vd, "Index"), path, index), depth: at.depth + 1, errs: at.errs}, vd, side)),
		}
	}
	if c.Values != nil || len(c.Keys) > 0 {
		key, item := "key"+suffix, "item"+suffix
		keyPath := gocode.Call(gocode.Selector(vd, "Key"), path, key)
		loop := &LoopView{Index: key, Item: item, Range: gocode.Call(gocode.Selector(vd, "SortedKeys"), deref), Map: deref}
		if len(c.Keys) > 0 {
			loop.Key = &CheckView{Errs: at.errs, Path: keyPath, Value: key, Deref: key}
			for _, r := range c.Keys {
				loop.Key.Calls = append(loop.Key.Calls, ruleCall(r, vd, key))
			}
		}
		if c.Values != nil {
			loop.Check = new(checkView(c.Values, checkAt{value: item, path: keyPath, depth: at.depth + 1, errs: at.errs}, vd, side))
		}
		cv.Values = loop
	}
	return cv
}

// ruleCall is the call of the validation package that checks rule r on value.
func ruleCall(r gomodel.Rule, vd, value string) string {
	if r.IsBase64 {
		value = gocode.Call(gocode.Selector(vd, "Base64"), value)
	}
	args := []string{value}
	switch r.Kind {
	case gomodel.RulePattern:
		args = append(args, r.Pattern.Name, gocode.RawString(r.Pattern.Text))
	case gomodel.RuleFormat:
		args = append(args, gocode.Quote(r.Format))
	case gomodel.RuleMinimum, gomodel.RuleMaximum:
		args = append(args, r.Number, strconv.FormatBool(r.IsExclusive))
	case gomodel.RuleConst:
		args = append(args, gocode.Literal(r.Const))
	case gomodel.RuleEnum:
		for _, v := range r.Values {
			args = append(args, gocode.Literal(v))
		}
	case gomodel.RuleEnumJSON:
		for _, v := range r.Values {
			args = append(args, gocode.RawString(string(jsonschema.Marshal(v))))
		}
	case gomodel.RuleUnique, gomodel.RuleUniqueJSON:
	case gomodel.RuleMinLength, gomodel.RuleMaxLength, gomodel.RuleMultipleOf, gomodel.RuleMinItems,
		gomodel.RuleMaxItems, gomodel.RuleMinProperties, gomodel.RuleMaxProperties:
		args = append(args, r.Number)
	}
	return gocode.Call(gocode.Selector(vd, ruleFuncs[r.Kind]), args...)
}

func errorView(d *gomodel.Decl, s *gocode.Scope) *ErrorView {
	return &ErrorView{
		Receiver:       receiver(d.Name),
		Runtime:        s.Import(gomodel.Import{Path: gomodel.RuntimePath}),
		Path:           d.Error.Path,
		PathLiteral:    gocode.Quote(d.Error.Path),
		Fallback:       gocode.Quote(d.Name),
		HasConstructor: d.Error.HasConstructor,
	}
}

func patternViews(patterns []*gomodel.Pattern, s *gocode.Scope) []PatternView {
	var out []PatternView
	for _, p := range patterns {
		out = append(out, PatternView{
			Name:   p.Name,
			Regexp: s.Import(gomodel.Import{Path: "regexp"}),
			Source: gocode.RawString(p.Source),
		})
	}
	return out
}
