// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package routes

import (
	"context"
	"testing"

	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
)

type service struct{}

func (service) Search(_ context.Context, opts *SearchServiceRequestOptions) (*SearchResponseData, error) {
	return NewSearchResponseData(new("found " + opts.Body.Text)), nil
}

func (service) PurgeSearch(context.Context, *PurgeSearchServiceRequestOptions) (*PurgeSearchResponseData, error) {
	return NewPurgeSearchResponseData(), nil
}

func (service) ListPets(context.Context, *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	return NewListPetsResponseData(new("pets")), nil
}

func (service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	return NewGetPetResponseData(new("pet " + opts.PathParams.PetID)), nil
}

func (service) GetFile(_ context.Context, opts *GetFileServiceRequestOptions) (*GetFileResponseData, error) {
	return NewGetFileResponseData(new("file " + opts.RawRequest.PathValue("rest"))), nil
}

func TestRouter(t *testing.T) {
	t.Parallel()

	servertest.Run(t, NewRouter(service{}), []servertest.Request{
		{Name: "QUERY of OpenAPI 3.2", Method: "QUERY", Path: "/search", Body: `{"text":"rex"}`, ContentType: "application/json", WantBody: `found rex`},
		{Name: "A method the spec adds", Method: "PURGE", Path: "/search", WantStatus: 204},
		{Name: "A method the path does not have", Path: "/search", WantStatus: 405, WantBody: "Method Not Allowed\n", WantHeaders: map[string]string{"Allow": "PURGE, QUERY"}},
		{Name: "A path that ends in a slash", Path: "/pets/", WantBody: `pets`},
		{Name: "A path that ends in a slash takes no path below it", Path: "/pets/rex/toys", WantStatus: 404, WantBody: "404 page not found\n"},
		{Name: "A path without its last slash is sent to the one with it", Path: "/pets", WantStatus: 307, WantBody: "<a href=\"/pets/\">Temporary Redirect</a>.\n\n", WantHeaders: map[string]string{"Location": "/pets/"}},
		{Name: "A parameter whose name is no identifier", Path: "/pets/rex", WantBody: `pet rex`},
		{Name: "An escaped path value arrives unescaped", Path: "/pets/a%2Fb", WantBody: `pet a/b`},
		{Name: "A GET route answers HEAD requests too", Method: "HEAD", Path: "/pets/rex", WantBody: `pet rex`},
		{Name: "The star takes the rest of the path", Path: "/files/a/b.txt", WantBody: `file a/b.txt`},
		{Name: "The rest of the path may be empty", Path: "/files/", WantBody: `file `},
		{Name: "The path before the star is sent to the one with the slash", Path: "/files", WantStatus: 307, WantBody: "<a href=\"/files/\">Temporary Redirect</a>.\n\n", WantHeaders: map[string]string{"Location": "/files/"}},
	})
}
