// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package nullable

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// store holds one pet and applies a patch to it: an absent property is kept, null clears it.
type store struct {
	pet    Pet
	notify runtime.Nullable[bool]
}

func (s *store) UpdatePet(_ context.Context, opts *UpdatePetServiceRequestOptions) (*UpdatePetResponseData, error) {
	p := opts.Body
	s.notify = opts.Query.Notify
	if name, ok := p.Name.Get(); ok {
		s.pet.Name = name
	}
	if p.Tag.IsSet() {
		s.pet.Tag = p.Tag
	}
	if p.Nickname.IsSet() {
		s.pet.Nickname = p.Nickname
	}
	if p.Age.IsSet() {
		s.pet.Age = p.Age
	}
	if p.Owner.IsSet() {
		s.pet.Owner = p.Owner
	}
	if p.Toys != nil {
		s.pet.Toys = p.Toys
	}
	return NewUpdatePetResponseData(&s.pet), nil
}

func TestPatchKeepsClearsAndSets(t *testing.T) {
	t.Parallel()

	s := newStore()
	srv := httptest.NewServer(NewRouter(s))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	got, err := c.UpdatePet(t.Context(), &UpdatePetRequestOptions{
		PathParams: &UpdatePetPathParams{ID: 1},
		Query:      &UpdatePetQuery{Notify: runtime.Some(false)},
		Body: &PetPatch{
			Tag:      runtime.Null[string](),
			Nickname: runtime.Null[string](),
			Age:      runtime.Some(4),
			Owner:    runtime.Null[Owner](),
			Toys:     []string{},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, "Rex", got.Name)
	assert.True(t, got.Tag.IsNull())
	assert.True(t, got.Nickname.IsNull())
	assert.Equal(t, runtime.Some(4), got.Age)
	assert.True(t, got.Owner.IsNull())
	assert.Equal(t, []string{}, got.Toys)
	assert.Equal(t, runtime.Some(false), s.notify)
}

func TestPatchJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(PetPatch{Nickname: runtime.Null[string](), Age: runtime.Some(4), Toys: []string{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"nickname":null,"age":4,"toys":[]}`, string(data))

	var p PetPatch
	require.NoError(t, json.Unmarshal([]byte(`{"nickname":null,"age":4}`), &p))
	assert.False(t, p.Name.IsSet())
	assert.True(t, p.Nickname.IsNull())
	age, ok := p.Age.Get()
	assert.True(t, ok)
	assert.Equal(t, 4, age)
	assert.Equal(t, "Rexy", p.Nickname.Or("Rexy"))
}

func TestRequiredNullableWritesNull(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(Pet{ID: 1, Name: "Rex"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":1,"name":"Rex","tag":null}`, string(data))
}

func TestValidateRejectsNull(t *testing.T) {
	t.Parallel()

	err := PetPatch{Age: runtime.Null[int](), Nickname: runtime.Some(strings.Repeat("x", 21))}.Validate()
	require.Error(t, err)
	assert.ErrorContains(t, err, "age: must not be null")
	assert.ErrorContains(t, err, "nickname: must be at most 20 characters long")
	assert.NoError(t, PetPatch{Nickname: runtime.Null[string]()}.Validate())
}

func TestServerRejectsNull(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(newStore()))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, srv.URL+"/pets/1", strings.NewReader(`{"age":null}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestLegacyStaysPointer(t *testing.T) {
	t.Parallel()

	legacy := "old"
	assert.Equal(t, &legacy, PetPatch{Legacy: &legacy}.Legacy)
}

func newStore() *store {
	return &store{pet: Pet{
		ID:       1,
		Name:     "Rex",
		Tag:      runtime.Some("dog"),
		Nickname: runtime.Some("Rexy"),
		Age:      runtime.Some(3),
		Owner:    runtime.Some(Owner{Name: "Ann"}),
		Toys:     []string{"ball"},
	}}
}
