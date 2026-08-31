// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package errors

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	return NewPutPetResponseData(opts.Body), nil
}

func TestErrors(t *testing.T) {
	t.Parallel()

	router := NewRouter(service{})
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "A pet", method: "GET", path: "/pets/1", wantStatus: 200, wantBody: `{"name":"Rex","age":3}`},
		{name: "The request fails validation", method: "GET", path: "/pets/0", wantStatus: 400, wantBody: `{"error":"invalid request: path.id: must be at least 1"}`},
		{name: "A query value outside the enum", method: "GET", path: "/pets/1?fields=color", wantStatus: 400, wantBody: `{"error":"invalid request: query.fields[0]: must be one of name, age"}`},
		{name: "The response fails validation", method: "GET", path: "/pets/2", wantStatus: 500, wantBody: `{"error":"invalid response: name: must be at least 1 characters long"}`},
		{name: "A typed error carries the status of its response", method: "GET", path: "/pets/9", wantStatus: 404, wantBody: `{"detail":"no pet 9"}`},
		{name: "A wrapped typed error", method: "GET", path: "/pets/4", wantStatus: 404, wantBody: `{"detail":"wrapped"}`},
		{name: "A typed error behind a pointer", method: "GET", path: "/pets/5", wantStatus: 404, wantBody: `{"detail":"pointer"}`},
		{name: "A response type that is not an error is a response", method: "GET", path: "/pets/3", wantStatus: 409, wantBody: `{"detail":"locked","until":"later"}`},
		{name: "A body that fails validation", method: "PUT", path: "/pets/1", body: `{"name":""}`, wantStatus: 400, wantBody: `{"error":"invalid request: body.name: must be at least 1 characters long"}`},
		{name: "A read-only field is checked in the response only", method: "PUT", path: "/pets/1", body: `{"name":"Rex","age":-1}`, wantStatus: 500, wantBody: `{"error":"invalid response: age: must be at least 0"}`},
		{name: "A valid body", method: "PUT", path: "/pets/1", body: `{"name":"Rex"}`, wantStatus: 200, wantBody: `{"name":"Rex"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}
