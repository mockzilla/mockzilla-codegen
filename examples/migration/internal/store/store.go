// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package store keeps the pets of the migration examples in memory.
package store

import (
	"maps"
	"slices"
	"sync"
)

// Store holds values by ID, safe for concurrent use.
type Store[T any] struct {
	mu   sync.Mutex
	byID map[int64]T
	last int64
}

// Get returns the value of id and whether there is one.
func (s *Store[T]) Get(id int64) (T, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.byID[id]
	return v, ok
}

// Add stores the value build returns for the next ID.
func (s *Store[T]) Add(build func(id int64) T) T {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byID == nil {
		s.byID = map[int64]T{}
	}
	s.last++
	v := build(s.last)
	s.byID[s.last] = v
	return v
}

// List returns at most limit values in ID order, all of them when limit is 0.
func (s *Store[T]) List(limit int) []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]T, 0, len(s.byID))
	for _, id := range slices.Sorted(maps.Keys(s.byID)) {
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, s.byID[id])
	}
	return out
}

// Delete removes the value of id.
func (s *Store[T]) Delete(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.byID, id)
}
