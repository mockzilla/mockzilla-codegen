// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

const (
	unvisited = iota
	onStack
	done
)

// use is the first part use that makes one folder import another.
type use struct {
	part PartID
	used PartID
}

// cycleFinder walks the imports between folders depth first.
type cycleFinder struct {
	edges map[string]map[string]use
	state map[string]int
	stack []string
}

// find returns the folders of the first cycle, the first folder repeated at the end.
func (c *cycleFinder) find() []string {
	for _, dir := range slices.Sorted(maps.Keys(c.edges)) {
		if c.state[dir] != unvisited {
			continue
		}
		if cycle := c.visit(dir); cycle != nil {
			return cycle
		}
	}
	return nil
}

func (c *cycleFinder) visit(dir string) []string {
	c.state[dir] = onStack
	c.stack = append(c.stack, dir)
	for _, next := range slices.Sorted(maps.Keys(c.edges[dir])) {
		if c.state[next] == onStack {
			i := slices.Index(c.stack, next)
			return append(slices.Clone(c.stack[i:]), next)
		}
		if c.state[next] != unvisited {
			continue
		}
		if cycle := c.visit(next); cycle != nil {
			return cycle
		}
	}
	c.stack = c.stack[:len(c.stack)-1]
	c.state[dir] = done
	return nil
}

// importCycle fails when parts in two folders use each other, directly or through others: Go
// forbids import cycles.
func importCycle(files []*File, byPart map[PartID]*File, parts []Part) error {
	edges := folderEdges(files, byPart, parts)
	cycle := (&cycleFinder{edges: edges, state: make(map[string]int, len(edges))}).find()
	if cycle == nil {
		return nil
	}

	shown := make(map[string]string, len(files))
	for _, f := range files {
		shown[filepath.Dir(f.Path)] = filepath.Dir(f.Rel)
	}
	folders := []string{shown[cycle[0]]}
	why := make([]string, 0, len(cycle)-1)
	for i := 1; i < len(cycle); i++ {
		folders = append(folders, shown[cycle[i]])
		u := edges[cycle[i-1]][cycle[i]]
		why = append(why, fmt.Sprintf("%s uses %s", u.part, u.used))
	}
	return fmt.Errorf("%w: %s (%s)", ErrImportCycle, strings.Join(folders, " -> "), strings.Join(why, ", "))
}

// folderEdges maps each folder to the folders its parts use.
func folderEdges(files []*File, byPart map[PartID]*File, parts []Part) map[string]map[string]use {
	uses := make(map[PartID][]PartID, len(parts))
	for _, p := range parts {
		uses[p.ID] = p.Uses
	}

	edges := make(map[string]map[string]use)
	for _, f := range files {
		from := filepath.Dir(f.Path)
		for _, id := range f.Parts {
			for _, u := range uses[id] {
				target := byPart[u]
				if target == nil || filepath.Dir(target.Path) == from {
					continue
				}
				to := filepath.Dir(target.Path)
				if edges[from] == nil {
					edges[from] = make(map[string]use)
				}
				if _, ok := edges[from][to]; !ok {
					edges[from][to] = use{part: id, used: u}
				}
			}
		}
	}
	return edges
}
