// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package framework

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

func TestHTTPHandler(t *testing.T) {
	t.Parallel()

	f := &layout.File{Path: "/work/gen.go", Package: "api"}
	s := gocode.NewScope(f, &layout.Layout{Files: []*layout.File{f}})

	assert.Equal(t, Handler{Signature: "(w http.ResponseWriter, r *http.Request)", Return: "return"}, HTTPHandler(s))
	assert.Equal(t, `import "net/http"`, s.Imports.Decl())
}

func TestParams(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"owner", "id"}, Params("/owners/{owner}/pets/{id}.json"))
	assert.Nil(t, Params("/pets"))
}

func TestShape(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "GET /owners/{}/pets/{}", Shape("GET /owners/{owner}/pets/{id}"))
	assert.Equal(t, "/files/{}", Shape("/files/{path...}"))
}

func TestConflictsByShape(t *testing.T) {
	t.Parallel()

	get := Route{Operation: "GetPet", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	del := Route{Operation: "DeletePet", Method: "DELETE", Path: "/pets/{petId}", Pattern: "/pets/:petId"}
	again := Route{Operation: "GetPetAgain", Method: "GET", Path: "/pets/{id}", Pattern: "/pets/:id"}
	renamed := Route{Operation: "GetAnimal", Method: "GET", Path: "/pets/{animalId}", Pattern: "/pets/:animalId"}

	kept, dropped := ConflictsByShape([]Route{get, del, again, renamed})

	assert.Equal(t, []Route{get, del}, kept)
	assert.Equal(t, []Conflict{
		{Route: again, Reason: "repeats the route of GetPet"},
		{Route: renamed, Reason: "names its path parameters otherwise than GetPet at /pets/{id}"},
	}, dropped)
}
