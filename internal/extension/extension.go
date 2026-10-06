// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package extension reads the x-* extensions mockzilla-codegen knows into typed values.
package extension

import (
	"cmp"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// Names of the extensions Parse reads.
const (
	GoType           = "x-go-type"
	GoTypeImport     = "x-go-type-import"
	GoTypeName       = "x-go-type-name"
	GoName           = "x-go-name"
	GoNameExact      = "x-go-name-exact"
	SkipPointer      = "x-go-type-skip-optional-pointer"
	Nullable         = "x-go-nullable"
	JSONIgnore       = "x-go-json-ignore"
	OmitEmpty        = "x-omitempty"
	ExtraTags        = "x-go-extra-tags"
	EnumNames        = "x-enum-names"
	DeprecatedReason = "x-deprecated-reason"
	SensitiveData    = "x-sensitive-data"
	MCPName          = "x-mcp"
)

// MaskKind is how a sensitive value is masked.
type MaskKind int

const (
	MaskFull MaskKind = iota
	MaskRegex
	MaskHash
	MaskPartial
)

// typoPrefix starts the names this generator owns: an unknown name with it is likely a typo.
const typoPrefix = "x-go-"

// maskNames are the mask names x-sensitive-data takes, in MaskKind order.
var maskNames = []string{"full", "regex", "hash", "partial"}

// Set holds the extensions of one schema, parameter or operation. Fields of extensions that are
// absent keep their zero value.
type Set struct {
	GoType           *Type
	TypeName         string
	Name             string
	IsExactName      bool
	IsPointerSkipped bool
	Nullable         *bool
	IsJSONIgnored    bool
	OmitEmpty        *bool
	Tags             []Tag
	EnumNames        []string
	DeprecatedReason string
	Sensitive        *Mask
	MCP              *MCP
}

// Type is x-go-type: Name as written, such as uuid.UUID or string, and the package x-go-type-import
// names, with the name to import it under.
type Type struct {
	Name  string
	Path  string
	Alias string
}

// Tag is one struct tag of x-go-extra-tags.
type Tag struct {
	Key   string
	Value string
}

// Mask is x-sensitive-data. Pattern is the regular expression of MaskRegex; KeepPrefix and
// KeepSuffix are the characters MaskPartial leaves visible.
type Mask struct {
	Kind       MaskKind
	Pattern    string
	KeepPrefix int
	KeepSuffix int
}

// MCP is x-mcp on an operation.
type MCP struct {
	Skip        *bool
	Name        string
	Description string
}

// IsSkipped applies x-mcp.skip over the default of the config; a nil MCP is an operation without
// x-mcp.
func (m *MCP) IsSkipped(defaultSkip bool) bool {
	if m != nil && m.Skip != nil {
		return *m.Skip
	}
	return defaultSkip
}

// reader parses the extensions of one place and reports what it cannot use.
type reader struct {
	at    spec.Origin
	diags []diag.Diagnostic
}

// Parse reads exts, found at the given place. A value of the wrong type is left out with a warning,
// and so is an unknown name that looks like one of ours.
func Parse(exts []spec.Extension, at spec.Origin) (Set, []diag.Diagnostic) {
	r := &reader{at: at}
	var s Set
	var goType, goImport *spec.Value
	for _, e := range exts {
		v := e.Value
		switch e.Name {
		case GoType:
			goType = &v
		case GoTypeImport:
			goImport = &v
		case GoTypeName:
			s.TypeName = r.identifier(e.Name, v)
		case GoName:
			s.Name = r.identifier(e.Name, v)
		case GoNameExact:
			s.IsExactName = r.boolean(e.Name, v)
		case SkipPointer:
			s.IsPointerSkipped = r.boolean(e.Name, v)
		case Nullable:
			if b, ok := r.optionalBool(e.Name, v); ok {
				s.Nullable = &b
			}
		case JSONIgnore:
			s.IsJSONIgnored = r.boolean(e.Name, v)
		case OmitEmpty:
			if b, ok := r.optionalBool(e.Name, v); ok {
				s.OmitEmpty = &b
			}
		case ExtraTags:
			s.Tags = r.tags(v)
		case EnumNames:
			s.EnumNames = r.enumNames(v)
		case DeprecatedReason:
			s.DeprecatedReason = r.text(e.Name, v)
		case SensitiveData:
			s.Sensitive = r.mask(v)
		case MCPName:
			s.MCP = r.mcp(v)
		default:
			if strings.HasPrefix(e.Name, typoPrefix) {
				r.warn(diag.CodeExtensionUnknown, e.Name, "unknown extension "+e.Name+"; it is left out")
			}
		}
	}
	s.GoType = r.goType(goType, goImport)
	return s, r.diags
}

func (r *reader) warn(code, name, message string) {
	r.diags = append(r.diags, diag.Diagnostic{
		Severity: diag.Warning,
		Code:     code,
		Pointer:  r.at.Pointer + "/" + name,
		Origin:   diag.Origin{File: r.at.File, Line: r.at.Line, Col: r.at.Col},
		Message:  message,
	})
}

func (r *reader) wrong(name, want string) {
	r.warn(diag.CodeExtensionValue, name, name+" must be "+want+"; it is left out")
}

func (r *reader) text(name string, v spec.Value) string {
	if v.Kind != spec.KindString {
		r.wrong(name, "a string")
		return ""
	}
	return v.Str
}

// identifier is a Go identifier as written.
func (r *reader) identifier(name string, v spec.Value) string {
	if v.Kind != spec.KindString || !token.IsIdentifier(v.Str) {
		r.wrong(name, "a Go identifier")
		return ""
	}
	return v.Str
}

func (r *reader) boolean(name string, v spec.Value) bool {
	b, _ := r.optionalBool(name, v)
	return b
}

// optionalBool reads a boolean, or a string that holds one as older specs write it.
func (r *reader) optionalBool(name string, v spec.Value) (bool, bool) {
	switch v.Kind {
	case spec.KindBool:
		return v.Bool, true
	case spec.KindString:
		if b, err := strconv.ParseBool(v.Str); err == nil {
			return b, true
		}
	case spec.KindNull, spec.KindNumber, spec.KindArray, spec.KindObject:
	}
	r.wrong(name, "a boolean")
	return false, false
}

func (r *reader) tags(v spec.Value) []Tag {
	if v.Kind != spec.KindObject {
		r.wrong(ExtraTags, "an object of strings")
		return nil
	}
	var out []Tag
	for _, f := range v.Fields {
		if f.Value.Kind != spec.KindString {
			r.wrong(ExtraTags, "an object of strings")
			return nil
		}
		out = append(out, Tag{Key: f.Name, Value: f.Value.Str})
	}
	slices.SortFunc(out, func(a, b Tag) int { return cmp.Compare(a.Key, b.Key) })
	return out
}

func (r *reader) enumNames(v spec.Value) []string {
	if v.Kind != spec.KindArray {
		r.wrong(EnumNames, "a list of Go identifiers")
		return nil
	}
	out := make([]string, len(v.Items))
	for i, item := range v.Items {
		if item.Kind != spec.KindString || !token.IsIdentifier(item.Str) {
			r.wrong(EnumNames, "a list of Go identifiers")
			return nil
		}
		out[i] = item.Str
	}
	return out
}

// mask reads true, a mask name, or an object with mask, pattern, keepPrefix and keepSuffix.
func (r *reader) mask(v spec.Value) *Mask {
	const want = "true, a mask name (full, regex, hash, partial) or an object"
	switch v.Kind {
	case spec.KindBool:
		if v.Bool {
			return &Mask{}
		}
		return nil
	case spec.KindString:
		if k := MaskKind(slices.Index(maskNames, v.Str)); k >= 0 && k != MaskRegex {
			return &Mask{Kind: k}
		}
	case spec.KindObject:
		return r.maskObject(v, want)
	case spec.KindNull, spec.KindNumber, spec.KindArray:
	}
	r.wrong(SensitiveData, want)
	return nil
}

func (r *reader) maskObject(v spec.Value, want string) *Mask {
	m := &Mask{}
	for _, f := range v.Fields {
		isValid := true
		switch f.Name {
		case "mask":
			m.Kind = MaskKind(slices.Index(maskNames, f.Value.Str))
			isValid = m.Kind >= 0
		case "pattern":
			m.Pattern, isValid = f.Value.Str, f.Value.Kind == spec.KindString
		case "keepPrefix":
			m.KeepPrefix, isValid = count(f.Value)
		case "keepSuffix":
			m.KeepSuffix, isValid = count(f.Value)
		}
		if !isValid {
			r.wrong(SensitiveData, want)
			return nil
		}
	}
	if m.Kind == MaskRegex && m.Pattern == "" {
		r.wrong(SensitiveData, "an object with a pattern for the regex mask")
		return nil
	}
	return m
}

func (r *reader) mcp(v spec.Value) *MCP {
	if v.Kind != spec.KindObject {
		r.wrong(MCPName, "an object")
		return nil
	}
	m := &MCP{}
	for _, f := range v.Fields {
		switch f.Name {
		case "skip":
			if b, ok := r.optionalBool(MCPName+".skip", f.Value); ok {
				m.Skip = &b
			}
		case "name":
			m.Name = r.text(MCPName+".name", f.Value)
		case "description":
			m.Description = r.text(MCPName+".description", f.Value)
		}
	}
	return m
}

// goType reads x-go-type and the import x-go-type-import gives it: an object with path and name, or
// the path alone.
func (r *reader) goType(typ, imp *spec.Value) *Type {
	if typ == nil {
		if imp != nil {
			r.warn(diag.CodeExtensionValue, GoTypeImport, GoTypeImport+" needs "+GoType+"; it is left out")
		}
		return nil
	}
	if typ.Kind != spec.KindString || typ.Str == "" {
		r.wrong(GoType, "a Go type")
		return nil
	}

	t := &Type{Name: typ.Str}
	switch {
	case imp == nil:
	case imp.Kind == spec.KindString:
		t.Path = imp.Str
	case imp.Kind == spec.KindObject:
		for _, f := range imp.Fields {
			switch strings.ToLower(f.Name) {
			case "path":
				t.Path = f.Value.Str
			case "name":
				t.Alias = f.Value.Str
			}
		}
	default:
		r.wrong(GoTypeImport, "a path or an object with path and name")
	}
	return t
}

// count reads a number of characters: a whole number from 0.
func count(v spec.Value) (int, bool) {
	n, err := strconv.Atoi(v.Num.String())
	return n, v.Kind == spec.KindNumber && err == nil && n >= 0
}
