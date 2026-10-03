// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package layout places generated parts in files and gives each folder its package name and
// import path.
package layout

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

// PartID names one piece of generated code, such as models.types.
type PartID string

// Part is a part a generator writes, with the parts its code refers to. Package, when set, is the
// package name of the file that holds it, whatever its folder is called: main for a program.
// Owner, when set, is the part whose types this one adds methods to, so both must share a folder.
type Part struct {
	ID      PartID
	Uses    []PartID
	Package string
	Owner   PartID
}

type FileKind int

const (
	Generated FileKind = iota
	Scaffold
)

// The parts of the scaffold files, each written to the file the config names for it.
const (
	PartScaffoldService    PartID = "server.scaffold.service"
	PartScaffoldMiddleware PartID = "server.scaffold.middleware"
	PartScaffoldMain       PartID = "server.scaffold.main"
)

// File is one output file. Path is resolved against the config folder; Rel is the path as the
// config writes it. Parts keep the order the generators gave.
type File struct {
	Path       string
	Rel        string
	Package    string
	ImportPath string
	Parts      []PartID
	Kind       FileKind
}

// Layout holds the files that got at least one part, sorted by path. Package is the package of
// output.file, which is not among them when every part went elsewhere.
type Layout struct {
	Files   []*File
	Package string

	byPart map[PartID]*File
}

// candidate is a file the config names, with the selectors that move parts to it.
type candidate struct {
	file      *File
	selectors []string
}

// scaffold is a starter file the config asks for, and the part that fills it.
type scaffold struct {
	rel  string
	part PartID
}

// Plan puts every part in the file whose selector matches it most closely, output.file when
// none does, then names the package and import path of every folder.
func Plan(cfg *config.Config, parts []Part, mod Module) (*Layout, error) {
	def, cands := candidates(cfg)
	l := &Layout{byPart: make(map[PartID]*File, len(parts))}
	used, err := l.assign(def, cands, parts)
	if err != nil {
		return nil, err
	}
	if err = unknownSelectors(cands, used, parts); err != nil {
		return nil, err
	}

	l.collect(def, cands)
	if err = l.name(cfg, mod); err != nil {
		return nil, err
	}
	if err = l.sameFolder(parts); err != nil {
		return nil, err
	}

	if err = importCycle(l.Files, folderEdges(l.Files, l.byPart, parts)); err != nil {
		return nil, err
	}
	return l, nil
}

// FileOf returns the file that holds p, or nil for a part the layout does not know.
func (l *Layout) FileOf(p PartID) *File {
	return l.byPart[p]
}

// CheckImports fails when the folders import each other in a cycle through what the parts wrote,
// which Plan cannot see for the code of an extra file. imports lists, by part, the import paths
// its file held once the part was written, so a path counts for the first part of a file that has
// it.
func (l *Layout) CheckImports(imports map[PartID][]string) error {
	return importCycle(l.Files, importEdges(l.Files, imports))
}

// assign places every part and returns the selectors that placed one.
func (l *Layout) assign(def *File, cands []candidate, parts []Part) (map[string]bool, error) {
	used := make(map[string]bool)
	for _, p := range parts {
		best, rank := def, 0
		var rival *File
		for _, c := range cands {
			for _, sel := range c.selectors {
				if !selects(sel, p.ID) {
					continue
				}
				used[sel] = true
				switch r := strings.Count(sel, ".") + 1; {
				case r > rank:
					best, rank, rival = c.file, r, nil
				case r == rank && c.file != best:
					rival = c.file
				}
			}
		}
		if rival != nil {
			return nil, fmt.Errorf("%w: %s is selected by both %s and %s", ErrSelectorConflict, p.ID, best.Rel, rival.Rel)
		}
		l.byPart[p.ID] = best
		best.Parts = append(best.Parts, p.ID)
		if p.Package != "" {
			best.Package = p.Package
		}
	}
	return used, nil
}

// sameFolder checks that every part with an owner is in the owner's folder, since methods must
// be declared in the package of their type.
func (l *Layout) sameFolder(parts []Part) error {
	for _, p := range parts {
		if p.Owner == "" {
			continue
		}
		f, owner := l.byPart[p.ID], l.byPart[p.Owner]
		if filepath.Dir(f.Path) != filepath.Dir(owner.Path) {
			return fmt.Errorf("%w: %s adds methods to the types of %s, so %s must be in the folder of %s", ErrSplitParts, p.ID, p.Owner, f.Rel, owner.Rel)
		}
	}
	return nil
}

func (l *Layout) collect(def *File, cands []candidate) {
	seen := make(map[*File]bool, len(cands)+1)
	for _, f := range append([]*File{def}, candidateFiles(cands)...) {
		if len(f.Parts) > 0 && !seen[f] {
			seen[f] = true
			l.Files = append(l.Files, f)
		}
	}
	slices.SortFunc(l.Files, func(a, b *File) int { return cmp.Compare(a.Path, b.Path) })
}

// name sets the package and import path of every file, and the package of output.file.
// output.packages wins, then package for the folder of output.file, then the folder name.
func (l *Layout) name(cfg *config.Config, mod Module) error {
	names := make(map[string]string, len(cfg.Output.Packages)+1)
	for dir, pkg := range cfg.Output.Packages {
		names[resolve(cfg, dir)] = pkg
	}
	defDir := filepath.Dir(resolve(cfg, cfg.Output.File))
	if _, ok := names[defDir]; !ok {
		names[defDir] = cmp.Or(cfg.Package, naming.Package(defDir))
	}
	l.Package = names[defDir]

	dirs := make([]string, 0, len(l.Files))
	for _, f := range l.Files {
		dirs = append(dirs, filepath.Dir(f.Path))
	}
	dirs = slices.Compact(slices.Sorted(slices.Values(dirs)))
	imports, err := importPaths(dirs, mod)
	if err != nil {
		return err
	}

	packages := make(map[string]*File, len(dirs))
	for _, f := range l.Files {
		dir := filepath.Dir(f.Path)
		f.ImportPath = imports[dir]
		switch pkg, ok := names[dir]; {
		case f.Package != "":
		case ok:
			f.Package = pkg
		default:
			f.Package = naming.Package(dir)
		}

		if other, ok := packages[dir]; ok && other.Package != f.Package {
			return fmt.Errorf("%w: %s is package %s next to %s, package %s", ErrPackageConflict, f.Rel, f.Package, other.Rel, other.Package)
		}
		packages[dir] = f
	}
	return nil
}

// candidates are output.file, the files output.files names, and the scaffold files and extra
// files, each with its own part. The part of an extra file is named by its path.
func candidates(cfg *config.Config) (*File, []candidate) {
	def := &File{Path: resolve(cfg, cfg.Output.File), Rel: cfg.Output.File}
	list := make([]candidate, 0, len(cfg.Output.Files)+len(cfg.ExtraFiles))
	for _, rel := range slices.Sorted(maps.Keys(cfg.Output.Files)) {
		f := &File{Path: resolve(cfg, rel), Rel: rel}
		if f.Path == def.Path {
			f = def
		}
		list = append(list, candidate{file: f, selectors: cfg.Output.Files[rel]})
	}
	for _, sc := range scaffolds(cfg) {
		f := &File{Path: resolve(cfg, sc.rel), Rel: sc.rel, Kind: Scaffold}
		list = append(list, candidate{file: f, selectors: []string{string(sc.part)}})
	}
	for _, rel := range slices.Sorted(maps.Keys(cfg.ExtraFiles)) {
		list = append(list, candidate{file: &File{Path: resolve(cfg, rel), Rel: rel}, selectors: []string{rel}})
	}
	return def, list
}

// scaffolds lists the scaffold files server.scaffold names, in a fixed order.
func scaffolds(cfg *config.Config) []scaffold {
	if cfg.Server == nil {
		return nil
	}
	var out []scaffold
	for _, sc := range []scaffold{
		{rel: cfg.Server.Scaffold.Service, part: PartScaffoldService},
		{rel: cfg.Server.Scaffold.Middleware, part: PartScaffoldMiddleware},
		{rel: cfg.Server.Scaffold.Main, part: PartScaffoldMain},
	} {
		if sc.rel != "" {
			out = append(out, sc)
		}
	}
	return out
}

func candidateFiles(cands []candidate) []*File {
	files := make([]*File, len(cands))
	for i, c := range cands {
		files[i] = c.file
	}
	return files
}

func resolve(cfg *config.Config, p string) string {
	return filepath.Clean(cfg.Resolve(p))
}

// selects reports whether sel names id or a group id belongs to.
func selects(sel string, id PartID) bool {
	return string(id) == sel || strings.HasPrefix(string(id), sel+".")
}

func unknownSelectors(cands []candidate, used map[string]bool, parts []Part) error {
	var unknown []string
	for _, c := range cands {
		for _, sel := range c.selectors {
			if !used[sel] {
				unknown = append(unknown, fmt.Sprintf("%q in %s", sel, c.file.Rel))
			}
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	known := make([]string, len(parts))
	for i, p := range parts {
		known[i] = string(p.ID)
	}
	return fmt.Errorf("%w %s; the parts are %s", ErrUnknownSelector, strings.Join(unknown, ", "), strings.Join(known, ", "))
}

// importPaths gives each folder its import path. Output in one folder needs none, so there a
// missing module or a folder outside it is fine.
func importPaths(dirs []string, mod Module) (map[string]string, error) {
	paths := make(map[string]string, len(dirs))
	if mod.Path == "" {
		if len(dirs) > 1 {
			return nil, fmt.Errorf("%w: the output spans %d folders, which import each other by module path; add a go.mod or set output.module", ErrNoModule, len(dirs))
		}
		return paths, nil
	}

	for _, dir := range dirs {
		rel, err := filepath.Rel(mod.Dir, dir)
		if err != nil || !filepath.IsLocal(rel) {
			if len(dirs) > 1 {
				return nil, fmt.Errorf("%w: %s is not inside %s", ErrOutsideModule, dir, mod.Dir)
			}
			continue
		}
		paths[dir] = path.Join(mod.Path, filepath.ToSlash(rel))
	}
	return paths, nil
}
