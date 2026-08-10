// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package itest holds the parts of the integration test that can be unit tested: collecting
// specs, the sandbox module, running and building jobs, the result cache, known failures and the
// report.
package itest

import (
	"cmp"
	"errors"
	"fmt"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const stashFolder = "stash"

var specExtensions = []string{".yml", ".yaml", ".json"}

// Spec is one spec file a run generates from. Name is the path relative to the specs folder, or
// to the repo when the file is outside it, with forward slashes.
type Spec struct {
	Name string
	Path string
	Size int64
}

// Collect returns the named specs, or every spec under dir when none is named, largest first so
// the slow ones start early. A name is a path relative to repo or to dir. The walk leaves out
// stash folders and names starting with "-". A missing dir gives no specs.
func Collect(repo, dir string, named []string) ([]Spec, error) {
	var specs []Spec
	var err error
	if len(named) > 0 {
		specs, err = find(repo, dir, named)
	} else {
		specs, err = walk(dir)
	}
	if err != nil {
		return nil, err
	}

	slices.SortFunc(specs, func(a, b Spec) int {
		return cmp.Or(cmp.Compare(b.Size, a.Size), cmp.Compare(a.Name, b.Name))
	})
	return specs, nil
}

// SafeNames returns a Go package name for each spec name, in the same order. Names that collide
// get a number, handed out in sorted order so the result does not depend on the input order.
func SafeNames(names []string) []string {
	sorted := slices.Sorted(slices.Values(names))
	taken := make(map[string]bool, len(names))
	byName := make(map[string]string, len(names))
	for _, n := range sorted {
		if _, isDone := byName[n]; isDone {
			continue
		}
		base := safeName(n)
		name := base
		for i := 2; taken[name]; i++ {
			name = base + "_" + strconv.Itoa(i)
		}
		taken[name] = true
		byName[n] = name
	}

	out := make([]string, len(names))
	for i, n := range names {
		out[i] = byName[n]
	}
	return out
}

func find(repo, dir string, named []string) ([]Spec, error) {
	var specs []Spec
	seen := map[string]bool{}
	for _, n := range named {
		candidates := []string{filepath.Join(repo, n), filepath.Join(dir, n)}
		if filepath.IsAbs(n) {
			candidates = []string{filepath.Clean(n)}
		}

		p, info := firstFile(candidates)
		switch {
		case info == nil:
			return nil, fmt.Errorf("%w: %s", ErrSpecNotFound, n)
		case seen[p]:
			continue
		}
		seen[p] = true
		specs = append(specs, Spec{Name: specName(p, dir, repo), Path: p, Size: info.Size()})
	}
	return specs, nil
}

func walk(dir string) ([]Spec, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	var specs []Spec
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		var info fs.FileInfo
		if err == nil {
			info, err = d.Info()
		}
		switch {
		case err != nil:
			return err
		case info.IsDir() && p != dir && isSkipped(info.Name()):
			return filepath.SkipDir
		case info.IsDir() || isSkipped(info.Name()) || !slices.Contains(specExtensions, filepath.Ext(p)):
			return nil
		}
		specs = append(specs, Spec{Name: specName(p, dir), Path: p, Size: info.Size()})
		return nil
	})
	return specs, err
}

// firstFile returns the first of paths that is a regular file, and its info; nil info when none is.
func firstFile(paths []string) (string, fs.FileInfo) {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p, info
		}
	}
	return "", nil
}

func isSkipped(name string) bool {
	return name == stashFolder || strings.HasPrefix(name, "-")
}

// specName is p relative to the first of bases that holds it, else p itself.
func specName(p string, bases ...string) string {
	for _, base := range bases {
		if rel, err := filepath.Rel(base, p); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

// safeName lowers name, drops its extension and joins its runs of letters and digits with "_".
func safeName(name string) string {
	name = strings.TrimSuffix(name, path.Ext(name))
	var b strings.Builder
	isGap := false
	for _, r := range strings.ToLower(name) {
		if 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			if isGap && b.Len() > 0 {
				b.WriteByte('_')
			}
			isGap = false
			b.WriteRune(r)
			continue
		}
		isGap = true
	}

	s := b.String()
	if s == "" || '0' <= s[0] && s[0] <= '9' {
		s = "s" + s
	}
	// A package named main needs a func main; init cannot name a package.
	if token.IsKeyword(s) || s == "main" || s == "init" {
		s += "_"
	}
	return s
}
