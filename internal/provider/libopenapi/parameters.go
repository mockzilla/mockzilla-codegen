// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The parameters of a path item merged with those of an operation: on the same name and location,
// the operation's own wins.

package libopenapi

import (
	"slices"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func mergeParameters(shared, own []*spec.Parameter) []*spec.Parameter {
	if len(shared) == 0 {
		return own
	}

	out := slices.Clone(shared)
	for _, p := range own {
		i := slices.IndexFunc(out[:len(shared)], func(s *spec.Parameter) bool { return s.In == p.In && s.Name == p.Name })
		if i < 0 {
			out = append(out, p)
			continue
		}
		out[i] = p
	}
	return out
}
