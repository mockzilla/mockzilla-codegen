// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"errors"
	"strings"

	"github.com/pb33f/libopenapi/index"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/spec"
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

// buildIssues takes cycles from the index, which lists them all, not from the build error.
func (c *converter) buildIssues(err error, cycles []*index.CircularReferenceResult) {
	for _, e := range flatten(err) {
		if re := (*index.ResolvingError)(nil); errors.As(e, &re) && re.CircularReference != nil {
			continue
		}
		c.diags.Append(diag.Diagnostic{Severity: diag.Warning, Code: diag.CodeBuildIssue, Origin: diag.Origin{File: c.file}, Message: e.Error()})
	}

	for _, cr := range cycles {
		d := diag.Diagnostic{
			Severity: diag.Info,
			Code:     diag.CodeCircularRef,
			Origin:   diag.Origin{File: c.file},
			Message:  "circular reference: " + cr.GenerateJourneyPath(),
		}
		// libopenapi ignores nullable when it calls a loop infinite, so real specs trip it: warn only.
		if cr.IsInfiniteLoop {
			d.Severity, d.Code = diag.Warning, diag.CodeInfiniteCircularRef
			d.Message = "circular reference with every step required: " + cr.GenerateJourneyPath()
		}
		if at := cr.LoopPoint; at != nil {
			d.Pointer = refPointer(at.Definition)
			d.Origin = c.position(d.Pointer, at.Node)
		}
		c.diags.Append(d)
	}
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
