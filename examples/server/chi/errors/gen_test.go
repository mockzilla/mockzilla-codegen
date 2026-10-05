// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package errors

import (
	"context"
	"fmt"
	"testing"

	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
)

// service answers by id: 1 is a pet, 2 a pet the spec rejects, 3 is locked, 4 fails with a
// wrapped Problem, 5 with a Problem behind a pointer, anything else with a Problem.
type service struct{}

func (service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	switch id := opts.PathParams.ID; id {
	case 1:
		return NewGetPetResponseData200(&Pet{Name: "Rex", Age: new(3)}), nil
	case 2:
		return NewGetPetResponseData200(&Pet{Name: ""}), nil
	case 3:
		return NewGetPetResponseData409(&Locked{Detail: new("locked"), Until: new("later")}), nil
	case 4:
		return nil, fmt.Errorf("lookup: %w", NewProblem("wrapped"))
	case 5:
		return nil, &Problem{Detail: "pointer"}
	default:
		return nil, NewProblem(fmt.Sprintf("no pet %d", id))
	}
}

func (service) PutPet(_ context.Context, opts *PutPetServiceRequestOptions) (*PutPetResponseData, error) {
	return NewPutPetResponseData200(opts.Body), nil
}

func TestErrors(t *testing.T) {
	t.Parallel()

	servertest.Run(t, NewRouter(service{}), servertest.Errors)
}
