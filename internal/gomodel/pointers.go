// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// When a field or value is a pointer, and what lies behind pointers and aliases.

package gomodel

import "strings"

// presence is what decides whether a struct field is a pointer.
type presence struct {
	isRequired       bool
	isNullable       bool
	isPointerSkipped bool
	isInCycle        bool
}

// Held is how a struct field holds a value of type t that may be absent: a pointer, unless t can
// be nil already.
func Held(t Type) Type {
	return elemType(t, true)
}

// Elem is the type a pointer points to, or t itself.
func Elem(t Type) Type {
	if p, ok := t.(Pointer); ok {
		return p.Elem
	}
	return t
}

// Validates reports whether a value of type t, or of the type t points to, has a Validate method.
func Validates(t Type) bool {
	d, ok := validated(Elem(t))
	return ok && (d == nil || d.Validation != nil)
}

// Underlying is the type under t, through aliases and defined types.
func Underlying(t Type) Type {
	for {
		r, ok := t.(DeclRef)
		if !ok || r.Decl.Kind != KindAlias && r.Decl.Kind != KindDefined {
			return t
		}
		t = r.Decl.Target
	}
}

// StructDecl is the struct declaration a value of type t is, through pointers and aliases, or nil.
func StructDecl(t Type) *Decl {
	if r, ok := unalias(Elem(t)).(DeclRef); ok && r.Decl.Kind == KindStruct {
		return r.Decl
	}
	return nil
}

// ErrorDecl is the error type a value of type t is, through pointers and aliases, or nil.
func ErrorDecl(t Type) *Decl {
	for {
		switch x := t.(type) {
		case Pointer:
			t = x.Elem
		case DeclRef:
			if x.Decl.Error != nil {
				return x.Decl
			}
			if x.Decl.Kind != KindAlias && x.Decl.Kind != KindDefined {
				return nil
			}
			t = x.Decl.Target
		default:
			return nil
		}
	}
}

// fieldType makes a field a pointer when it can be absent or null, or when its type holds the
// struct itself by value. Types that can be nil already stay as they are.
func fieldType(t Type, p presence) Type {
	switch {
	case nilable(t):
		return t
	case p.isInCycle, p.isRequired && p.isNullable:
		return Pointer{Elem: t}
	case p.isPointerSkipped, p.isRequired:
		return t
	}
	return Pointer{Elem: t}
}

// elemType is for array items and map values: a pointer only when the value can be null.
func elemType(t Type, isNullable bool) Type {
	if isNullable && !nilable(t) {
		return Pointer{Elem: t}
	}
	return t
}

// nilable reports types whose zero value is nil: slices, maps, pointers, any, json.RawMessage.
// Alias cycles must be broken before, or this does not return.
func nilable(t Type) bool {
	switch t := t.(type) {
	case Slice, Map, Pointer:
		return true
	case Builtin:
		// x-go-type can name a slice, map or pointer type as written.
		return t == anyType || strings.HasPrefix(t.Name, "[]") || strings.HasPrefix(t.Name, "map[") || strings.HasPrefix(t.Name, "*")
	case Qualified:
		return t == rawJSON
	case DeclRef:
		d := t.Decl
		return (d.Kind == KindAlias || d.Kind == KindDefined) && nilable(d.Target)
	}
	return false
}
