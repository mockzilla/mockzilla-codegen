// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

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
)

// Validate checks the config and returns a *ValidationError listing every problem, or nil.
// A spec path is not required here: it can come from the command line or from memory.
func (c *Config) Validate() error {
	issues := slices.Concat(
		checkPackage("package", c.Package),
		checkImports(c.Imports),
		checkSimplify(c.Spec.Simplify),
		checkModels(c.Models),
		checkServer(c.Server),
		checkMCP(c.MCP, c.Client),
		checkOutput(c.Output),
	)
	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: issues}
}

func checkPackage(key, name string) []Issue {
	if token.IsIdentifier(name) {
		return nil
	}
	return []Issue{{Key: key, Message: fmt.Sprintf("%q is not a valid Go package name", name)}}
}

func checkImports(imports []Import) []Issue {
	var issues []Issue
	for i, imp := range imports {
		if imp.Package == "" {
			issues = append(issues, Issue{Key: fmt.Sprintf("imports[%d].package", i), Message: "required"})
		}
	}
	return issues
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
	return checkEnum("server.framework", s.Framework, frameworks)
}

func checkMCP(m *MCP, cl *Client) []Issue {
	if m == nil || cl != nil {
		return nil
	}
	return []Issue{{Key: "mcp", Message: "needs a client block"}}
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
