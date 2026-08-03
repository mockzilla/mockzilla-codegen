// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ImportSet holds the imports of one file and the name each is used under.
type ImportSet struct {
	names map[string]string
	paths map[string]string
}

func NewImportSet() *ImportSet {
	return &ImportSet{names: make(map[string]string), paths: make(map[string]string)}
}

// Add imports path and returns the name code uses for it. want is the preferred name; empty means
// the package name guessed from the path. A name another path holds gets the first free number.
func (s *ImportSet) Add(path, want string) string {
	if name, ok := s.names[path]; ok {
		return name
	}

	base := want
	if base == "" {
		base = guessName(path)
	}
	name := base
	for i := 2; s.paths[name] != ""; i++ {
		name = base + strconv.Itoa(i)
	}
	s.names[path] = name
	s.paths[name] = path
	return name
}

// Has reports whether path is imported.
func (s *ImportSet) Has(path string) bool {
	_, ok := s.names[path]
	return ok
}

// Decl returns the import declaration: the standard library first, then the rest, each group
// sorted by path. It is empty when nothing is imported.
func (s *ImportSet) Decl() string {
	var std, other []string
	for _, path := range slices.Sorted(maps.Keys(s.names)) {
		line := strconv.Quote(path)
		if name := s.names[path]; name != lastElem(path) {
			line = name + " " + line
		}
		if isStd(path) {
			std = append(std, line)
		} else {
			other = append(other, line)
		}
	}

	lines := slices.Concat(std, other)
	switch len(lines) {
	case 0:
		return ""
	case 1:
		return "import " + lines[0]
	}

	var b strings.Builder
	b.WriteString("import (\n")
	for i, line := range lines {
		if i == len(std) && i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("\t" + line + "\n")
	}
	b.WriteString(")")
	return b.String()
}

// guessName follows the usual layout of module paths: a major version suffix (v2) and a go-
// prefix are not part of the name, nor is anything after a dot (yaml.v3).
func guessName(path string) string {
	elems := strings.Split(path, "/")
	name := elems[len(elems)-1]
	if isMajorVersion(name) && len(elems) > 1 {
		name = elems[len(elems)-2]
	}
	name = strings.TrimPrefix(name, "go-")
	name, _, _ = strings.Cut(name, ".")
	name = strings.Map(func(r rune) rune {
		if r == '_' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, name)

	if !token.IsIdentifier(name) {
		return "pkg"
	}
	return name
}

func isMajorVersion(elem string) bool {
	n, err := strconv.Atoi(strings.TrimPrefix(elem, "v"))
	return strings.HasPrefix(elem, "v") && err == nil && n > 1
}

func lastElem(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

// isStd reports whether path is in the standard library: its first element has no dot.
func isStd(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}
