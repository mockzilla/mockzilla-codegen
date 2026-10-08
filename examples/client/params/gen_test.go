// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package params

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// serve starts a server that answers an empty object and passes on each request it gets.
func serve(t *testing.T) (*Client, <-chan *http.Request) {
	t.Helper()

	seen := make(chan *http.Request, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c, seen
}

func TestPathStyles(t *testing.T) {
	t.Parallel()

	c, seen := serve(t)

	_, err := c.PathStyles(context.Background(), &PathStylesRequestOptions{PathParams: &PathStylesPathParams{Simple: "a b", Label: 5, Matrix: true, List: []string{"a", "b"}}})

	require.NoError(t, err)
	assert.Equal(t, "/path/a%20b/.5/;matrix=true/a,b", (<-seen).URL.EscapedPath())
}

func TestQueryStyles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query *QueryStylesQuery
		want  string
	}{
		{
			name: "Every query style",
			query: &QueryStylesQuery{
				Form: []int{1, 2}, Csv: []string{"a", "b"}, Space: []string{"a", "b"}, Pipe: []string{"a", "b"},
				Deep: &Point{X: new(1), Y: new(2)}, Flat: &Point{X: new(3), Y: new(4)}, JSON: &Point{X: new(5)}, Needed: "yes",
			},
			want: "form=1&form=2&csv=a,b&space=a%20b&pipe=a%7Cb&deep%5Bx%5D=1&deep%5By%5D=2&flat=x,3,y,4&json=%7B%22x%22%3A5%7D&needed=yes",
		},
		{
			name: "A deep object with a list and an object inside",
			query: &QueryStylesQuery{
				Filter: &Filter{Name: new("a"), Tags: []string{"b", "c"}, Size: &Point{X: new(1)}}, Needed: "yes",
			},
			want: "filter%5Bname%5D=a&filter%5Btags%5D=b&filter%5Btags%5D=c&filter%5Bsize%5D%5Bx%5D=1&needed=yes",
		},
		{
			name:  "An object whose list is unset",
			query: &QueryStylesQuery{Filter: &Filter{Name: new("a"), Tags: []string{}}, Needed: "yes"},
			want:  "filter%5Bname%5D=a&needed=yes",
		},
		{
			name:  "An exploded object writes a list as its key once per item",
			query: &QueryStylesQuery{Queries: &QueryStylesQueryQueries{Reference: new("r"), Status: []string{"a", "b"}}, Needed: "yes"},
			want:  "reference=r&status=a&status=b&needed=yes",
		},
		{
			name:  "A union writes the variant that is set",
			query: &QueryStylesQuery{ID: &QueryStylesQueryID{String: new("a7")}, Needed: "yes"},
			want:  "id=a7&needed=yes",
		},
		{
			name:  "A space and a comma inside an item are escaped, reserved characters kept where allowed",
			query: &QueryStylesQuery{Csv: []string{"a b", "c,d"}, IDs: new("List(1,2)"), Needed: "x y"},
			want:  "csv=a%20b,c%2Cd&ids=List(1,2)&needed=x%20y",
		},
		{
			name:  "Query parameters left out stay out",
			query: &QueryStylesQuery{Needed: "yes"},
			want:  "needed=yes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, seen := serve(t)

			_, err := c.QueryStyles(context.Background(), &QueryStylesRequestOptions{Query: tc.query})

			require.NoError(t, err)
			r := <-seen
			assert.Equal(t, "/query", r.URL.Path)
			assert.Equal(t, tc.want, r.URL.RawQuery)
		})
	}
}

func TestHeaderStyles(t *testing.T) {
	t.Parallel()

	c, seen := serve(t)

	_, err := c.HeaderStyles(context.Background(), &HeaderStylesRequestOptions{Headers: &HeaderStylesHeaders{
		XTags: []string{"a", "b"}, XPoint: &Point{X: new(1), Y: new(2)}, XWhen: new(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
		XLimit: &HeaderStylesHeadersXLimit{Int: new(5)},
	}})

	require.NoError(t, err)
	r := <-seen
	assert.Equal(t, "a,b", r.Header.Get("X-Tags"))
	assert.Equal(t, "x,1,y,2", r.Header.Get("X-Point"))
	assert.Equal(t, "2026-01-02T03:04:05Z", r.Header.Get("X-When"))
	assert.Equal(t, "5", r.Header.Get("X-Limit"))
}

func TestCookieStyles(t *testing.T) {
	t.Parallel()

	c, seen := serve(t)

	_, err := c.CookieStyles(context.Background(), &CookieStylesRequestOptions{Cookies: &CookieStylesCookies{
		Session: new("a b!"), Flags: []int{1, 2}, Prefs: []string{"dark%20mode", "wide"},
	}})

	require.NoError(t, err)
	r := <-seen
	session, err := r.Cookie("session")
	require.NoError(t, err)
	assert.Equal(t, "a%20b%21", session.Value, "a form cookie is percent-encoded")
	flags, err := r.Cookie("flags")
	require.NoError(t, err)
	assert.Equal(t, "1,2", flags.Value)
	prefs, err := r.Cookie("prefs")
	require.NoError(t, err)
	assert.Equal(t, "dark%20mode,wide", prefs.Value, "a cookie-style cookie goes as it is")
}

func TestMissingParameters(t *testing.T) {
	t.Parallel()

	c, err := NewClient("http://api.example.test")
	require.NoError(t, err)
	ctx := context.Background()

	req, err := c.QueryStylesRequest(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{}})
	require.NoError(t, err, "a required string that is empty is still sent")
	assert.Equal(t, "http://api.example.test/query?needed=", req.URL.String())

	_, err = c.PathStylesRequest(ctx, &PathStylesRequestOptions{PathParams: &PathStylesPathParams{Simple: "a"}})
	require.ErrorIs(t, err, runtime.ErrParamMissing, "a required list that is nil is missing")

	_, err = c.PathStylesRequest(ctx, nil)
	require.EqualError(t, err, "parameter is required: simple", "a path group left out leaves its placeholders")

	_, err = c.QueryStylesRequest(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{ID: &QueryStylesQueryID{}}})
	require.EqualError(t, err, "invalid parameter value: cannot write null as text", "a union with no variant set")
}

func TestQueryString(t *testing.T) {
	t.Parallel()

	c, seen := serve(t)
	ctx := context.Background()
	filter := &Filter{Name: new("rex & co"), Tags: []string{"a", "b"}}

	_, err := c.Search(ctx, &SearchRequestOptions{Filter: filter})
	require.NoError(t, err)
	assert.Equal(t, "name=rex%20%26%20co&tags=a&tags=b", (<-seen).URL.RawQuery, "a form")

	_, err = c.Find(ctx, &FindRequestOptions{Q: &Filter{Name: new("rex")}})
	require.NoError(t, err)
	assert.Equal(t, "%7B%22name%22%3A%22rex%22%7D", (<-seen).URL.RawQuery, "JSON, percent-encoded")

	_, err = c.Find(ctx, nil)
	require.ErrorIs(t, err, runtime.ErrParamMissing)
}
