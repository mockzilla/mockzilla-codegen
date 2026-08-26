// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

func TestPatternSet(t *testing.T) {
	t.Parallel()

	var diags diag.Collector
	p := newPatternSet(naming.New(nil), &diags)
	at := spec.Origin{Pointer: "/components/schemas/A", Line: 4, Col: 2}

	first := p.add(PartTypes, `^\d+$`, at, "PetID")
	same := p.add(PartTypes, `^\d+$`, at, "OwnerID")
	other := p.add(PartParams, `^\d+$`, at, "PetID")
	escaped := p.add(PartTypes, "^"+`\`+"u00e9", at, "Accent")
	assert.Nil(t, p.add(PartTypes, `(?=a)`, at, "Ahead"))
	assert.Nil(t, p.add(PartTypes, `(?=a)`, at, "Ahead"))

	assert.Same(t, first, same)
	assert.NotSame(t, first, other)
	assert.Equal(t, []*Pattern{first, other, escaped}, p.named())
	assert.Equal(t, []string{"patternPetID", "patternPetID2", "patternAccent"}, []string{first.Name, other.Name, escaped.Name})
	assert.Equal(t, `^\x{00e9}`, escaped.Source)
	assert.Len(t, diags.List(), 1)
}
