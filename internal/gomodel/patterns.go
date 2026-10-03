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
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

// unicodeEscape is \uXXXX, which RE2 writes as \x{XXXX}.
var unicodeEscape = regexp.MustCompile(`\\u([0-9a-fA-F]{4})`)

// patternKey finds the variable of a pattern within one part.
type patternKey struct {
	part   string
	source string
}

// patternSet holds the regular expressions generated code compiles, one variable per source and
// part, named once every one is known.
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

// add returns the variable of pattern in part, wanting the name want. A pattern RE2 cannot compile
// gives nil and a warning, once per place.
func (p *patternSet) add(part, pattern string, at spec.Origin, want string) *Pattern {
	source := unicodeEscape.ReplaceAllString(pattern, `\x{$1}`)
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

	key := patternKey{part: part, source: source}
	if found, ok := p.byKey[key]; ok {
		return found
	}
	found := &Pattern{Source: source, Part: part, Origin: origin(at)}
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
