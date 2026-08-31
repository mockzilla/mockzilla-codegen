// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

func TestScaffoldMainWithoutMiddleware(t *testing.T) {
	t.Parallel()

	m := petModel()
	opts := allOptions()
	opts.Scaffold.Middleware = false
	g, _ := New(m, opts)
	f := fixture{m: m, g: g, cfg: "output: {file: ./api/gen.go, module: example.com/work}\nserver:\n  framework: chi\n" +
		"  scaffold: {service: ./svc/service.go, main: ./cmd/server/main.go}\n"}

	main := string(f.render(t, layout.PartScaffoldMain))

	assert.Contains(t, main, "NewRouter(svc.NewPets())")
	assert.NotContains(t, main, "WithMiddleware")
}
