// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"fmt"
	"go/token"
	"maps"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var (
	frameworks      = enumOf(reflect.TypeFor[Server](), "Framework")
	intTypes        = enumOf(reflect.TypeFor[Models](), "IntType")
	selectorGroups  = []string{"models", "server", "client", "mcp", "plugin"}
	selectorSegment = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

	// templatePath matches template text that can only be a path: one word of path characters
	// with a slash in it. No text a block takes looks like that.
	templatePath = regexp.MustCompile(`^[\w.-]*(/[\w.-]+)+$`)
)

func checkPackage(key, name string) []Issue {
	if token.IsIdentifier(name) {
		return nil
	}
	return []Issue{{Key: key, Message: fmt.Sprintf("%q is not a valid Go package name", name)}}
}

// checkImports wants every import to be sound on its own and no name twice: code tells two
// packages apart by their names alone. A path can be listed once under a name and once under _.
func checkImports(imports []Import) []Issue {
	var issues []Issue
	named := make(map[string]int, len(imports))
	blank := make(map[string]int, len(imports))
	names := make(map[string]int, len(imports))
	for i, imp := range imports {
		entry := fmt.Sprintf("imports[%d]", i)
		if key, problem := importProblem(imp); problem != "" {
			issues = append(issues, Issue{Key: entry + "." + key, Message: problem})
			continue
		}

		paths := named
		if imp.Alias == blankAlias {
			paths = blank
		}
		if first, ok := paths[imp.Package]; ok {
			issues = append(issues, Issue{Key: entry + ".package", Message: fmt.Sprintf("%q is already listed in imports[%d]", imp.Package, first)})
			continue
		}
		paths[imp.Package] = i

		name := imp.Name()
		if first, ok := names[name]; ok {
			issues = append(issues, Issue{Key: entry + ".alias", Message: fmt.Sprintf("%q is the name of imports[%d] too, name one of them differently", name, first)})
			continue
		}
		if name != "" {
			names[name] = i
		}
	}
	return issues
}

// importProblem is the key of imp that is wrong and what is wrong with it, if anything.
func importProblem(imp Import) (key, problem string) {
	switch {
	case imp.Package == "":
		return "package", "required"
	case imp.Alias == dotAlias:
		return "alias", `"." is not supported: Go rejects a file that imports a package without using it, and under . the use cannot be checked`
	case imp.Alias != "" && !token.IsIdentifier(imp.Alias):
		return "alias", fmt.Sprintf("%q is not a valid Go identifier", imp.Alias)
	case imp.Alias == "" && imp.Name() == "":
		return "alias", fmt.Sprintf("required, %q does not end in a Go name", imp.Package)
	}
	return "", ""
}

func checkSimplify(s *Simplify) []Issue {
	if s == nil || s.OptionalProperties == nil {
		return nil
	}

	const key = "spec.simplify.optional-properties"
	op := s.OptionalProperties

	var issues []Issue
	if op.Min < 0 {
		issues = append(issues, Issue{Key: key + ".min", Message: "must not be negative"})
	}
	if op.Max < op.Min {
		issues = append(issues, Issue{Key: key + ".max", Message: fmt.Sprintf("must be at least min (%d)", op.Min)})
	}
	return issues
}

func checkModels(m *Models) []Issue {
	if m == nil {
		return nil
	}
	return checkEnum("models.int-type", m.IntType, intTypes)
}

func checkServer(s *Server) []Issue {
	if s == nil {
		return nil
	}
	issues := checkEnum("server.framework", s.Framework, frameworks)
	if s.Scaffold.Main != "" && s.Scaffold.Service == "" {
		issues = append(issues, Issue{Key: "server.scaffold.main", Message: "needs server.scaffold.service, which main starts"})
	}
	return issues
}

func checkMCP(m *MCP, cl *Client) []Issue {
	if m == nil || cl != nil {
		return nil
	}
	return []Issue{{Key: "mcp", Message: "needs a client block"}}
}

func checkTemplates(templates map[string]Template) []Issue {
	var issues []Issue
	for _, name := range slices.Sorted(maps.Keys(templates)) {
		if problem := templateProblem(templates[name]); problem != "" {
			issues = append(issues, Issue{Key: "templates." + name, Message: problem})
		}
	}
	return issues
}

// checkExtraFiles checks that every extra file is a Go file no other key of c writes.
func checkExtraFiles(c *Config) []Issue {
	type writer struct{ key, path string }
	writers := []writer{{key: "output.file", path: c.Output.File}}
	for _, p := range slices.Sorted(maps.Keys(c.Output.Files)) {
		writers = append(writers, writer{key: "output.files", path: p})
	}
	if s := c.Server; s != nil {
		writers = append(writers,
			writer{key: "server.scaffold.service", path: s.Scaffold.Service},
			writer{key: "server.scaffold.middleware", path: s.Scaffold.Middleware},
			writer{key: "server.scaffold.main", path: s.Scaffold.Main},
		)
	}
	taken := make(map[string]string, len(writers))
	for _, w := range writers {
		if clean := filepath.Clean(w.path); w.path != "" && taken[clean] == "" {
			taken[clean] = w.key
		}
	}

	paths := slices.Sorted(maps.Keys(c.ExtraFiles))
	issues := checkSamePaths("extra-files", "file", paths)
	for _, p := range paths {
		key := fmt.Sprintf("extra-files[%q]", p)
		if filepath.Ext(p) != ".go" {
			issues = append(issues, Issue{Key: key, Message: "is no .go file"})
		}
		if other := taken[filepath.Clean(p)]; other != "" {
			issues = append(issues, Issue{Key: key, Message: "is written by " + other + " too"})
		}
		if problem := templateProblem(c.ExtraFiles[p]); problem != "" {
			issues = append(issues, Issue{Key: key, Message: problem})
		}
	}
	return issues
}

// templateProblem is what is wrong with t, if anything. A template says by its form whether it
// is text or a file, so text that can only be a path is a file that is not given as one.
func templateProblem(t Template) string {
	text := strings.TrimSpace(t.Text)
	switch {
	case t.File != "" && t.Text != "":
		return "takes the text or a file, not both"
	case t.File == "" && text == "":
		return "is empty, want the text or {file: <path>}"
	case templatePath.MatchString(text):
		return fmt.Sprintf("%q is a path, want {file: %s}", text, text)
	}
	return ""
}

func checkEnum(key, value string, allowed []string) []Issue {
	if slices.Contains(allowed, value) {
		return nil
	}

	list := strings.Join(allowed, ", ")
	if value == "" {
		return []Issue{{Key: key, Message: "required, one of " + list}}
	}
	return []Issue{{Key: key, Message: fmt.Sprintf("%q is not one of %s", value, list)}}
}

func checkOutput(o Output) []Issue {
	files := slices.Sorted(maps.Keys(o.Files))
	dirs := slices.Sorted(maps.Keys(o.Packages))

	issues := slices.Concat(
		checkSamePaths("output.files", "file", files),
		checkSelectors(o.Files, files),
		checkSamePaths("output.packages", "folder", dirs),
	)
	for _, dir := range dirs {
		issues = append(issues, checkPackage(fmt.Sprintf("output.packages[%q]", dir), o.Packages[dir])...)
	}
	return issues
}

func checkSamePaths(key, what string, paths []string) []Issue {
	var issues []Issue
	seen := make(map[string]string, len(paths))
	for _, p := range paths {
		clean := filepath.Clean(p)
		if first, ok := seen[clean]; ok {
			issues = append(issues, Issue{Key: key, Message: fmt.Sprintf("%q and %q are the same %s", first, p, what)})
			continue
		}
		seen[clean] = p
	}
	return issues
}

// Whether a part exists is known only at generation time, so this checks syntax and repeats.
func checkSelectors(files map[string][]string, sorted []string) []Issue {
	var issues []Issue
	listedIn := make(map[string]string)
	for _, file := range sorted {
		key := fmt.Sprintf("output.files[%q]", file)
		for _, sel := range files[file] {
			if !validSelector(sel) {
				issues = append(issues, Issue{
					Key:     key,
					Message: fmt.Sprintf("invalid selector %q, want a form like models, models.types or plugin.<name>.<part>", sel),
				})
				continue
			}
			if first, ok := listedIn[sel]; ok {
				issues = append(issues, Issue{Key: key, Message: fmt.Sprintf("%q is already listed in %q", sel, first)})
				continue
			}
			listedIn[sel] = file
		}
	}
	return issues
}

func validSelector(sel string) bool {
	segments := strings.Split(sel, ".")
	depth := 2
	if segments[0] == "plugin" {
		depth = 3
	}
	if len(segments) > depth || !slices.Contains(selectorGroups, segments[0]) {
		return false
	}

	for _, s := range segments[1:] {
		if !selectorSegment.MatchString(s) {
			return false
		}
	}
	return true
}
