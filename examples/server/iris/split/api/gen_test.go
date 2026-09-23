// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package api_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/examples/server/iris/split/api"
	"github.com/mockzilla/mockzilla-codegen/examples/server/iris/split/books"
	"github.com/mockzilla/mockzilla-codegen/examples/server/iris/split/models"
)

// shelf holds one book, on top of the scaffolded service.
type shelf struct {
	*books.Books
}

func (shelf) GetBook(_ context.Context, opts *api.GetBookServiceRequestOptions) (*api.GetBookResponseData, error) {
	if opts.PathParams.Isbn != "1" {
		return api.NewGetBookResponseData404(&models.Problem{Detail: "no such book"}), nil
	}
	return api.NewGetBookResponseData200(&models.Book{Isbn: "1", Title: "Go"}), nil
}

func TestAcrossPackages(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(shelf{Books: books.NewBooks()}, api.WithMiddleware(books.RequestIDMiddleware), api.WithErrorHandler(api.DefaultErrorHandler{}))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/books/1", nil))
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, `{"isbn":"1","title":"Go"}`, rec.Body.String())
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/books/2", nil))
	assert.Equal(t, 404, rec.Code)
	assert.Equal(t, `{"detail":"no such book"}`, rec.Body.String())
}
