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

func TestRouter(t *testing.T) {
	t.Parallel()

	servertest.Run(t, NewRouter(service{}), []servertest.Request{
		{Name: "QUERY of OpenAPI 3.2", Method: "QUERY", Path: "/search", Body: `{"text":"rex"}`, ContentType: "application/json", WantBody: `found rex`},
		{Name: "A method the spec adds", Method: "PURGE", Path: "/search", WantStatus: 204},
		{Name: "A method the path does not have", Path: "/search", WantStatus: 405, WantBody: "Method Not Allowed\n", WantHeaders: map[string]string{"Allow": "PURGE, QUERY"}},
	})
}
