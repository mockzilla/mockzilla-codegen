// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Go types as values: builtins, declared types, types of other packages, pointers, Nullables,
// slices and maps.

package gomodel

// RuntimePath is the import path of the helpers that generated code uses.
const RuntimePath = "github.com/mockzilla/mockzilla-codegen/pkg/runtime"

// Type is a Go type expression. It stays a value until rendering turns it into text.
type Type interface {
	isType()
}

// Builtin is a predeclared type such as string, int64, byte or any.
type Builtin struct {
	Name string
}

func (Builtin) isType() {}

// DeclRef is a type this model declares.
type DeclRef struct {
	Decl *Decl
}

func (DeclRef) isType() {}

// Qualified is a type from another package, such as time.Time.
type Qualified struct {
	Import Import
	Name   string
}

func (Qualified) isType() {}

type Pointer struct {
	Elem Type
}

func (Pointer) isType() {}

// Nullable is runtime.Nullable of Elem: a value that may be absent or null.
type Nullable struct {
	Elem Type
}

func (Nullable) isType() {}

type Slice struct {
	Elem Type
}

func (Slice) isType() {}

type Map struct {
	Key  Type
	Elem Type
}

func (Map) isType() {}

// Import is a package path and the alias to import it under, empty for none.
type Import struct {
	Path  string
	Alias string
}
