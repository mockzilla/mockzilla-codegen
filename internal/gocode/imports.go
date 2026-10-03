// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/naming"
)

// An import under blank or dot has no name: the first only runs the package, the second lets the
// file write the package's exported names without one.
const (
	blank = "_"
	dot   = "."

	// fallbackName is the name of a package whose path gives none.
	fallbackName = "pkg"
)

// ImportSet holds the imports of one file: the name each is used under, and the ones under _ or .
// that have none. A path on offer holds its name, and the file does not import it yet; it is
// denied when another path asked for that name and got a number.
type ImportSet struct {
	names   map[string]string
	paths   map[string]string
	unnamed map[string]string
	offered map[string]bool
	denied  map[string]bool
}

func NewImportSet() *ImportSet {
	return &ImportSet{
		names:   make(map[string]string),
		paths:   make(map[string]string),
		unnamed: make(map[string]string),
		offered: make(map[string]bool),
		denied:  make(map[string]bool),
	}
}

// Add imports path and returns the name code uses for it. want is the preferred name; empty means
// the package name guessed from the path. A name another path holds gets the first free number.
// With _ or . as want the import takes no name and want comes back.
func (s *ImportSet) Add(path, want string) string {
	if want == blank || want == dot {
		// A dot import runs the package too, so it stands in for a blank one.
		if s.unnamed[path] != dot {
			s.unnamed[path] = want
		}
		return want
	}

	if name, ok := s.names[path]; ok {
		delete(s.offered, path)
		return name
	}

	base := ImportName(path, want)
	name := base
	for i := 2; s.paths[name] != ""; i++ {
		if holder := s.paths[name]; s.offered[holder] {
			s.denied[holder] = true
		}
		name = base + strconv.Itoa(i)
	}
	s.names[path] = name
	s.paths[name] = path
	return name
}

// Offer holds for path the name Add would give it, for code the file may get from elsewhere. The
// file imports path once Add asks for it or Take finds the name in that code. With _ or . as
// want the path is imported right away.
func (s *ImportSet) Offer(path, want string) {
	if want == blank || want == dot {
		s.Add(path, want)
		return
	}

	if _, isNamed := s.names[path]; !isNamed {
		s.Add(path, want)
		s.offered[path] = true
	}
}

// Take imports every path on offer whose name src, declarations of the file, refers to.
func (s *ImportSet) Take(src []byte) {
	if len(s.offered) == 0 {
		return
	}

	used := packageNames(src)
	maps.DeleteFunc(s.offered, func(path string, _ bool) bool { return used[s.names[path]] })
}

// Idle lists, sorted, the paths that are still on offer and denied their name to another path:
// without them the file would name its imports otherwise.
func (s *ImportSet) Idle() []string {
	var idle []string
	for path := range s.denied {
		if s.offered[path] {
			idle = append(idle, path)
		}
	}
	slices.Sort(idle)
	return idle
}

// Name is the name path is imported under. It has none when the file imports it under _ or .
// alone, has it on offer, or does not import it.
func (s *ImportSet) Name(path string) (string, bool) {
	if s.offered[path] {
		return "", false
	}
	name, ok := s.names[path]
	return name, ok
}

// Has reports whether path is imported, under a name or without one.
func (s *ImportSet) Has(path string) bool {
	_, isNamed := s.Name(path)
	_, isUnnamed := s.unnamed[path]
	return isNamed || isUnnamed
}

// Paths lists what is imported, sorted.
func (s *ImportSet) Paths() []string {
	all := slices.AppendSeq(slices.Collect(maps.Keys(s.names)), maps.Keys(s.unnamed))
	all = slices.DeleteFunc(all, func(path string) bool { return !s.Has(path) })
	slices.Sort(all)
	return slices.Compact(all)
}

// Decl returns the import declaration: the standard library first, then the rest, each group
// sorted by path. It is empty when nothing is imported.
func (s *ImportSet) Decl() string {
	var std, other []string
	for _, path := range s.Paths() {
		if isStd(path) {
			std = append(std, s.specs(path)...)
		} else {
			other = append(other, s.specs(path)...)
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

// specs are the import lines of path: the one under . or _, then the one under its name. A name
// makes _ needless, while . stays next to it.
func (s *ImportSet) specs(path string) []string {
	quoted := strconv.Quote(path)
	name, isNamed := s.Name(path)

	var out []string
	if alias, ok := s.unnamed[path]; ok && (alias == dot || !isNamed) {
		out = append(out, alias+" "+quoted)
	}
	if !isNamed {
		return out
	}

	if name != lastElem(path) {
		quoted = name + " " + quoted
	}
	return append(out, quoted)
}

// ImportName is the name path is imported under: alias when given, else the package name guessed
// from the path.
func ImportName(path, alias string) string {
	return cmp.Or(alias, naming.ImportName(path), fallbackName)
}

func lastElem(path string) string {
	return path[strings.LastIndex(path, "/")+1:]
}

// isStd reports whether path is in the standard library: its first element has no dot.
func isStd(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// packageNames are the names src, declarations of a file, puts before a dot without declaring
// them: its packages. A local or a parameter named like a package is none. It is nil when src is
// no Go code.
func packageNames(src []byte) map[string]bool {
	// The parser resolves the names src declares, so Obj stays nil on a package.
	file, err := parser.ParseFile(token.NewFileSet(), "", clause+string(src), 0)
	if err != nil {
		return nil
	}

	names := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, isSelector := n.(*ast.SelectorExpr); isSelector {
			if x, isIdent := sel.X.(*ast.Ident); isIdent && x.Obj == nil {
				names[x.Name] = true
			}
		}
		return true
	})
	return names
}
