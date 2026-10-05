// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The error-mapping config: the path to the message of each error type.

package gomodel

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
)

// resolveErrors gives each type of mapping, keyed by Go name, the path to its message. A name that
// is no struct or union, or a path that leads nowhere, is a warning and gets no Error method.
func resolveErrors(decls []*Decl, mapping map[string]string, c *diag.Collector) {
	byName := make(map[string]*Decl, len(decls))
	for _, d := range decls {
		byName[d.Name] = d
	}

	for _, name := range slices.Sorted(maps.Keys(mapping)) {
		path := mapping[name]
		d := byName[name]
		if d == nil || d.Kind != KindStruct && d.Kind != KindUnion {
			c.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeErrorMapping,
				Message:  fmt.Sprintf("models.error-mapping names %s, which is no generated struct or union", name),
			})
			continue
		}

		isFound, isSettable := errorPath(DeclRef{Decl: d}, strings.Split(path, "."))
		if !isFound {
			c.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodeErrorMapping,
				Pointer:  d.ID,
				Origin:   d.Origin,
				Message:  fmt.Sprintf("%s has no property at %s; it gets no Error method", name, path),
			})
			continue
		}
		d.Error = &ErrorMessage{Path: path, HasConstructor: isSettable}
		if isSettable {
			d.Error.Unset = unsetRequired(DeclRef{Decl: d}, strings.Split(path, "."))
		}
	}
}

// errorPath follows segs, JSON names with [] for the first item of an array, from type t. A union
// is followed through its shared fields, else through each variant, one of which must have the
// rest; a message behind a union cannot be set by a constructor.
func errorPath(t Type, segs []string) (isFound, isSettable bool) {
	isThroughUnion := false
	for i, seg := range segs {
		name, isFirst := strings.CutSuffix(seg, "[]")
		var f *Field
		if r, ok := unalias(t).(DeclRef); ok && r.Decl.Kind == KindUnion {
			isThroughUnion = true
			if f = fieldNamed(r.Decl.Struct, name); f == nil {
				return slices.ContainsFunc(r.Decl.Union.Variants, func(v *Variant) bool {
					found, _ := errorPath(v.Type, segs[i:])
					return found
				}), false
			}
		} else if st := structOf(t); st != nil {
			f = fieldNamed(st, name)
		}
		if f == nil {
			return false, false
		}

		t = Elem(f.Type)
		if isFirst {
			s, ok := sliceOf(t)
			if !ok {
				return false, false
			}
			t = Elem(s.Elem)
		}
	}
	return true, !isThroughUnion && groupOf(t) == groupString
}

// unsetRequired lists the required properties next to each step of segs, a path through structs
// alone, which a message set at segs leaves empty.
func unsetRequired(t Type, segs []string) []string {
	var out []string
	prefix := ""
	for _, seg := range segs {
		name, isFirst := strings.CutSuffix(seg, "[]")
		st := structOf(t)
		for _, f := range st.Fields {
			if f.Required && f.JSONName != name {
				out = append(out, prefix+f.JSONName)
			}
		}

		prefix += seg + "."
		t = Elem(fieldNamed(st, name).Type)
		if isFirst {
			s, _ := sliceOf(t)
			t = Elem(s.Elem)
		}
	}
	return out
}

func fieldNamed(st *Struct, name string) *Field {
	if i := slices.IndexFunc(st.Fields, func(f *Field) bool { return f.JSONName == name }); i >= 0 {
		return st.Fields[i]
	}
	return nil
}

// sliceOf returns the slice t is, through aliases and named slice types.
func sliceOf(t Type) (Slice, bool) {
	t = unalias(t)
	if r, ok := t.(DeclRef); ok && r.Decl.Kind == KindDefined {
		t = r.Decl.Target
	}
	s, ok := t.(Slice)
	return s, ok
}
