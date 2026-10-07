// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package paths

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func TestRequests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tests := []struct {
		name string
		base string
		call func(c *Client) error
		want string
	}{
		{
			name: "The query of the path goes ahead of the query parameters",
			call: func(c *Client) error {
				return c.SearchPhotos(ctx, &SearchPhotosRequestOptions{Query: &SearchPhotosQuery{Text: new("red fox")}})
			},
			want: "GET /rest?method=photos.search&text=red%20fox",
		},
		{
			name: "Path parameters fill the query of the path",
			call: func(c *Client) error {
				return c.ListOrders(ctx, &ListOrdersRequestOptions{PathParams: &ListOrdersPathParams{End: runtime.NewDate(2026, time.October, 4), Page: 2}})
			},
			want: "GET /orders?end=2026-10-04&page=2",
		},
		{
			name: "A fragment is not sent",
			call: func(c *Client) error {
				return c.ShareFile(ctx, &ShareFileRequestOptions{PathParams: &ShareFilePathParams{ID: "a b"}})
			},
			want: "PUT /files/a%20b",
		},
		{
			name: "The parameter a fragment repeats is sent",
			call: func(c *Client) error {
				return c.ListUsers(ctx, &ListUsersRequestOptions{Query: &ListUsersQuery{Action: ListUsersQueryActionListUsers}})
			},
			want: "GET /?Action=ListUsers",
		},
		{
			name: "The query of the base URL goes first",
			base: "/v1?key=abc#top",
			call: func(c *Client) error {
				return c.SearchPhotos(ctx, &SearchPhotosRequestOptions{Query: &SearchPhotosQuery{Text: new("red fox")}})
			},
			want: "GET /v1/rest?key=abc&method=photos.search&text=red%20fox",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, seen := serve(t, tc.base)

			err := tc.call(c)

			require.NoError(t, err)
			assert.Equal(t, tc.want, <-seen)
		})
	}
}

func TestAPlaceholderNoPathParameterFills(t *testing.T) {
	t.Parallel()

	c, err := NewClient("http://api.example.test")
	require.NoError(t, err)

	err = c.Search(context.Background(), &SearchRequestOptions{Query: &SearchQuery{Query: "go"}})

	require.EqualError(t, err, "parameter is required: query")
}

// serve starts a server that answers 204 and passes on the method and the URI of each request.
// The client's base URL is the server's followed by base.
func serve(t *testing.T, base string) (*Client, <-chan string) {
	t.Helper()

	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Method + " " + r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL + base)
	require.NoError(t, err)
	return c, seen
}
