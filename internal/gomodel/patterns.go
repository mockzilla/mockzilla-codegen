// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The regular expressions of pattern checks, one package-level variable each.

package gomodel

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/ecma"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// patternKey finds the variable of a pattern within one part.
type patternKey struct {
	part string
	text string
}

// patternSet holds one variable per pattern and part, named once every one is known.
type patternSet struct {
	namer  *naming.Namer
	diags  *diag.Collector
	byKey  map[patternKey]*Pattern
	list   []*Pattern
	wants  []string
	warned map[string]bool
}

func newPatternSet(n *naming.Namer, diags *diag.Collector) *patternSet {
	return &patternSet{namer: n, diags: diags, byKey: map[patternKey]*Pattern{}, warned: map[string]bool{}}
}

// add returns the variable of pattern in part, or nil with a warning when Go cannot compile it.
func (p *patternSet) add(part, pattern string, at spec.Origin, want string) *Pattern {
	source := ecma.RE2(pattern)
	if _, err := regexp.Compile(source); err != nil {
		if key := at.Pointer + "\x00" + pattern; !p.warned[key] {
			p.warned[key] = true
			p.diags.Append(diag.Diagnostic{
				Severity: diag.Warning,
				Code:     diag.CodePatternUnsupported,
				Pointer:  at.Pointer,
				Origin:   origin(at),
				Message:  fmt.Sprintf("pattern %q is not RE2 (%v); it is not used", pattern, err),
			})
		}
		return nil
	}

	key := patternKey{part: part, text: pattern}
	if found, ok := p.byKey[key]; ok {
		return found
	}
	found := &Pattern{Text: pattern, Source: source, Part: part, Origin: origin(at)}
	p.byKey[key] = found
	p.list = append(p.list, found)
	p.wants = append(p.wants, p.namer.Unexported("pattern", want))
	return found
}

// named names every pattern and returns them in the order they were added.
func (p *patternSet) named() []*Pattern {
	reqs := make([]naming.Request, len(p.list))
	for i := range p.list {
		reqs[i] = naming.Request{ID: strconv.Itoa(i), Want: p.wants[i], Order: i}
	}
	res := naming.Resolve(nil, reqs)
	for i, found := range p.list {
		found.Name = res.Names[strconv.Itoa(i)]
	}
	return p.list
}
