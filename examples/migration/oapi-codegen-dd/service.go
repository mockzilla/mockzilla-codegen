// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The service after the move from oapi-codegen-dd, quoted by docs/migration/oapi-codegen-dd.md.

package petstore

import (
	"context"
	"net/http"
	"strconv"

	"github.com/mockzilla/mockzilla-codegen/examples/migration/internal/store"
)

var _ PetsInterface = (*Service)(nil)

// Service keeps the pets in memory.
type Service struct {
	pets store.Store[Pet]
}

// ListPets handles GET /pets.
func (s *Service) ListPets(_ context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	limit := 0
	if opts.Query.Limit != nil {
		limit = int(*opts.Query.Limit)
	}

	return NewListPetsResponseData200(s.pets.List(limit)), nil
}

// CreatePet handles POST /pets.
func (s *Service) CreatePet(_ context.Context, opts *CreatePetServiceRequestOptions) (*CreatePetResponseData, error) {
	p := s.pets.Add(func(id int64) Pet {
		return Pet{ID: id, Name: opts.Body.Name, Tag: opts.Body.Tag, Status: opts.Body.Status}
	})

	headers := CreatePetResponse201Headers{Location: new("/pets/" + strconv.FormatInt(p.ID, 10))}
	return NewCreatePetResponseData201(&p).WithTypedHeaders(headers), nil
}

// GetPet handles GET /pets/{id}.
func (s *Service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets.Get(opts.PathParams.ID)
	if !ok {
		return nil, &Error{Code: http.StatusNotFound, Message: "no such pet"}
	}
	return NewGetPetResponseData200(&p), nil
}

// DeletePet handles DELETE /pets/{id}.
func (s *Service) DeletePet(_ context.Context, opts *DeletePetServiceRequestOptions) (*DeletePetResponseData, error) {
	s.pets.Delete(opts.PathParams.ID)
	return NewDeletePetResponseData(), nil
}
