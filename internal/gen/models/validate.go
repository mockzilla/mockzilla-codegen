// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The data of validate.tmpl, check.tmpl and error.tmpl: the Validate methods of a declaration,
// each check a runtime call, and the Error method of an error type.

package models

import (
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/jsonschema"
)

// ruleFuncs are the runtime checks, by rule kind.
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
	gomodel.RuleEnum:          "OneOf",
	gomodel.RuleEnumJSON:      "OneOfJSON",
}

// ValidateView is what the Validate methods of a declaration need. Enum lists the constants an enum
// takes; Response is set when ValidateResponse checks something else than Validate.
type ValidateView struct {
	Receiver string
	Runtime  string
	Enum     []string
	Request  MethodView
	Response *MethodView
}

// MethodView is the body of one Validate method; Count and Discriminator are union checks.
type MethodView struct {
	Runtime       string
	Count         string
	Discriminator string
	Checks        []CheckView
}

// CheckView checks one value. Every text is a Go expression: Calls return an error that is added
// under Path, Deref is Value behind its pointer. IsGuarded makes the calls on a value that is not
// nil.
type CheckView struct {
	Path       string
	Value      string
	Deref      string
	IsGuarded  bool
	IsRequired bool
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

// checkAt is where a check is written: its value and error path as Go expressions, and how deep in
// loops it sits.
type checkAt struct {
	value string
	path  string
	depth int
}

func validateView(d *gomodel.Decl, s *gocode.Scope) *ValidateView {
	v := d.Validation
	rt := s.Import(gomodel.Import{Path: gomodel.RuntimePath})
	out := &ValidateView{Receiver: receiver(d.Name), Runtime: rt}
	if d.Enum != nil {
		for _, ev := range d.Enum.Values {
			out.Enum = append(out.Enum, ev.Name)
		}
	}

	out.Request = methodView(d, rt, methodSide{skip: gomodel.SideResponse})
	if v.HasResponse {
		response := methodView(d, rt, methodSide{skip: gomodel.SideRequest, isResponse: true})
		out.Response = &response
	}
	return out
}

func methodView(d *gomodel.Decl, rt string, side methodSide) MethodView {
	v := d.Validation
	r := receiver(d.Name)
	m := MethodView{Runtime: rt}
	if v.Count != "" {
		args := make([]string, len(d.Union.Variants))
		for i, vr := range d.Union.Variants {
			args[i] = gocode.NotNil(gocode.Selector(r, vr.Name))
		}
		m.Count = gocode.Call(gocode.Selector(rt, v.Count), args...)
	}

	if v.IsDiscriminated {
		m.Discriminator = gocode.Call(gocode.Selector(rt, "DiscriminatorError"), gocode.Call(gocode.Selector(r, "MarshalJSON")))
	}

	for _, c := range v.Checks {
		if c.Side == side.skip {
			continue
		}
		value := r
		if c.Field != "" {
			value = gocode.Selector(r, c.Field)
		}
		m.Checks = append(m.Checks, checkView(c, checkAt{value: value, path: gocode.Quote(c.Path), depth: 1}, rt, side))
	}
	return m
}

// checkView writes check c of the value at. Loop variables get the depth as a suffix below the
// first level.
func checkView(c *gomodel.Check, at checkAt, rt string, side methodSide) CheckView {
	value, path := at.value, at.path
	deref := value
	if c.IsPointer {
		deref = gocode.Deref(value)
	}
	cv := CheckView{Path: path, Value: value, Deref: deref, IsRequired: c.IsRequired}
	for _, r := range c.Rules {
		cv.Calls = append(cv.Calls, ruleCall(r, rt, deref))
	}
	if c.IsNested {
		method := "Validate"
		if side.isResponse && c.Nested != nil && c.Nested.Validation.HasResponse {
			method = "ValidateResponse"
		}
		cv.Calls = append(cv.Calls, gocode.Call(gocode.Selector(value, method)))
	}
	// Ranging over a nil slice or map is fine; only calls need the value.
	cv.IsGuarded = c.IsGuarded && len(cv.Calls) > 0

	suffix := ""
	if at.depth > 1 {
		suffix = strconv.Itoa(at.depth)
	}
	if c.Items != nil {
		index, item := "idx"+suffix, "item"+suffix
		cv.Items = &LoopView{
			Index: index,
			Item:  item,
			Range: deref,
			Check: new(checkView(c.Items, checkAt{value: item, path: gocode.Call(gocode.Selector(rt, "Index"), path, index), depth: at.depth + 1}, rt, side)),
		}
	}
	if c.Values != nil || len(c.Keys) > 0 {
		key, item := "key"+suffix, "item"+suffix
		keyPath := gocode.Call(gocode.Selector(rt, "Key"), path, key)
		loop := &LoopView{Index: key, Item: item, Range: gocode.Call(gocode.Selector(rt, "SortedKeys"), deref), Map: deref}
		if len(c.Keys) > 0 {
			loop.Key = &CheckView{Path: keyPath, Value: key, Deref: key}
			for _, r := range c.Keys {
				loop.Key.Calls = append(loop.Key.Calls, ruleCall(r, rt, key))
			}
		}
		if c.Values != nil {
			loop.Check = new(checkView(c.Values, checkAt{value: item, path: keyPath, depth: at.depth + 1}, rt, side))
		}
		cv.Values = loop
	}
	return cv
}

// ruleCall is the runtime call that checks rule r on value.
func ruleCall(r gomodel.Rule, rt, value string) string {
	if r.IsBase64 {
		value = gocode.Call(gocode.Selector(rt, "Base64"), value)
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
	return gocode.Call(gocode.Selector(rt, ruleFuncs[r.Kind]), args...)
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
