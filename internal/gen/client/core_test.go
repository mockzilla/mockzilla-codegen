// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
)

func TestCoreView(t *testing.T) {
	t.Parallel()

	m := &gomodel.Model{}
	f := fixture{m: m, g: New(m, allOptions()), cfg: "output: {file: ./gen.go}\n"}
	s := f.scope(t, PartCore)

	got := coreView(f.g, s)

	assert.Equal(t, &CoreView{
		Name:    "PetClient",
		Option:  "PetClientOption",
		Context: "context",
		HTTP:    "http",
		URL:     "url",
		Runtime: "runtime",
		Timeout: "5 * time.Second",
		User:    map[string]any{"owner": "platform"},
	}, got)
	assert.True(t, s.Imports.Has("time"))
}
