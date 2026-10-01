// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gocode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// An import under blank or dot has no name: the first only runs the package, the second lets the
// file write the package's exported names without one.
const (
	blank = "_"
	dot   = "."
)

// ImportSet holds the imports of one file: the name each is used under, and the ones under _ or .
// that have none.
type ImportSet struct {
	names   map[string]string
	paths   map[string]string
	unnamed map[string]string
}

func NewImportSet() *ImportSet {
	return &ImportSet{names: make(map[string]string), paths: make(map[string]string), unnamed: make(map[string]string)}
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
		return name
	}

	base := ImportName(path, want)
	name := base
	for i := 2; s.paths[name] != ""; i++ {
		name = base + strconv.Itoa(i)
	}
	s.names[path] = name
	s.paths[name] = path
	return name
}

// Has reports whether path is imported, under a name or without one.
func (s *ImportSet) Has(path string) bool {
	_, isNamed := s.names[path]
	_, isUnnamed := s.unnamed[path]
	return isNamed || isUnnamed
}

// Paths lists what is imported, sorted.
func (s *ImportSet) Paths() []string {
	all := slices.AppendSeq(slices.Collect(maps.Keys(s.names)), maps.Keys(s.unnamed))
	slices.Sort(all)
	return slices.Compact(all)
}

// Trim takes out every import whose name src, the code of the file, does not refer to. An import
// under _ or . has no name to look for and stays. Nothing is taken out when src is no Go code.
func (s *ImportSet) Trim(src []byte) {
	used, ok := packageNames(src)
	if !ok {
		return
	}

	for _, path := range slices.Sorted(maps.Keys(s.names)) {
		if name := s.names[path]; !used[name] {
			delete(s.names, path)
			delete(s.paths, name)
		}
	}
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
	name, isNamed := s.names[path]

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
	if alias != "" {
		return alias
	}
	return guessName(path)
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

	if !token.IsIdentifier(name) || name == blank {
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

// packageNames are the names src, declarations of a file, puts before a dot without declaring
// them: its packages. A local or a parameter named like a package is none. The second result is
// false when src is no Go code.
func packageNames(src []byte) (map[string]bool, bool) {
	// The parser resolves the names src declares, so Obj stays nil on a package.
	file, err := parser.ParseFile(token.NewFileSet(), "", "package p\n"+string(src), 0)
	if err != nil {
		return nil, false
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
	return names, true
}
