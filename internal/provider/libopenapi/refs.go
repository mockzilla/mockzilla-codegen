// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// What libopenapi tells about a $ref, and the JSON pointer and component name the IR uses for it.

package libopenapi

import (
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// reference is what libopenapi's low-level objects tell about a $ref they were loaded from.
type reference interface {
	IsReference() bool
	GetReference() string
}

// memoized builds a $ref once at its target; each usage gets a copy that shares the schemas.
func memoized[T any](memo map[string]*T, r reference, ptr string, build func(ptr string) *T) (*T, *spec.ComponentRef) {
	if out, ok := memo[ptr]; ok {
		return out, nil
	}
	if !r.IsReference() {
		out := build(ptr)
		memo[ptr] = out
		return out, nil
	}

	target := refPointer(r.GetReference())
	base, ok := memo[target]
	if !ok {
		base = build(target)
		memo[target] = base
	}
	cp := *base
	memo[ptr] = &cp
	return &cp, componentRef(target)
}

// refPointer turns a local $ref into the pointer form this package builds; other files stay as is.
func refPointer(ref string) string {
	if !strings.HasPrefix(ref, "#") {
		return ref
	}

	tokens, err := oasdoc.Split(ref)
	if err != nil {
		return strings.TrimPrefix(ref, "#")
	}
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString("/")
		b.WriteString(oasdoc.Escape(t))
	}
	return b.String()
}

// componentName is the component key when ptr is a component of this document, else "".
func componentName(ptr string) string {
	tokens, err := oasdoc.Split(ptr)
	if err != nil || len(tokens) != 3 || tokens[0] != "components" {
		return ""
	}
	return tokens[2]
}

func componentRef(ptr string) *spec.ComponentRef {
	return &spec.ComponentRef{Pointer: ptr, Name: componentName(ptr)}
}

func flatten(err error) []error {
	if err == nil {
		return nil
	}
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return []error{err}
	}

	var out []error
	for _, e := range joined.Unwrap() {
		out = append(out, flatten(e)...)
	}
	return out
}
