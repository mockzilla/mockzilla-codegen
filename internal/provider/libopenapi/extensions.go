// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package libopenapi

import (
	"github.com/pb33f/libopenapi/orderedmap"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/spec"
)

func extensions(m *orderedmap.Map[string, *yaml.Node]) []spec.Extension {
	var out []spec.Extension
	for name, n := range m.FromOldest() {
		out = append(out, spec.Extension{Name: name, Value: value(n)})
	}
	return out
}
