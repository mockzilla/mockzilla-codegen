// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Schema keywords libopenapi gives in its own form: types and examples.

package libopenapi

import (
	"strings"

	"github.com/pb33f/libopenapi/datamodel/high/base"

	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func typeOf(name string) spec.TypeSet {
	switch name {
	case "string":
		return spec.TypeString
	case "number":
		return spec.TypeNumber
	case "integer":
		return spec.TypeInteger
	case "boolean":
		return spec.TypeBoolean
	case "object":
		return spec.TypeObject
	case "array":
		return spec.TypeArray
	case "null":
		return spec.TypeNull
	default:
		return 0
	}
}

// examples prefers the 3.1 examples list and falls back to the single example.
func examples(h *base.Schema) []spec.Value {
	if len(h.Examples) > 0 {
		return values(h.Examples)
	}
	if h.Example != nil {
		return []spec.Value{value(h.Example)}
	}
	return nil
}

func isFileName(s string) bool {
	for _, ext := range []string{".yaml", ".yml", ".json"} {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}
