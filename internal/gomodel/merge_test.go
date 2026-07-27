// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/codegen/internal/spec"
)

func TestAllOfInModel(t *testing.T) {
	t.Parallel()
	checkGolden(t, "allof", "allof", testOptions())
}

func TestTypeSetText(t *testing.T) {
	t.Parallel()

	assert.Empty(t, typeSetText(0))
	assert.Equal(t, "string", typeSetText(spec.TypeString))
	assert.Equal(t, "number or integer or null", typeSetText(spec.TypeNumber|spec.TypeInteger|spec.TypeNull))
}
