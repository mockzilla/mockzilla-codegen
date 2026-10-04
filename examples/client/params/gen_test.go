// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package params

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// echo answers every operation with the parameters it received.
type echo struct{}

func (echo) PathStyles(_ context.Context, opts *PathStylesServiceRequestOptions) (*PathStylesResponseData, error) {
	return NewPathStylesResponseData(Echo{"path": opts.PathParams}), nil
}

func (echo) QueryStyles(_ context.Context, opts *QueryStylesServiceRequestOptions) (*QueryStylesResponseData, error) {
	return NewQueryStylesResponseData(Echo{"query": opts.Query}), nil
}

func (echo) HeaderStyles(_ context.Context, opts *HeaderStylesServiceRequestOptions) (*HeaderStylesResponseData, error) {
	return NewHeaderStylesResponseData(Echo{"header": opts.Headers}), nil
}

func (echo) CookieStyles(_ context.Context, opts *CookieStylesServiceRequestOptions) (*CookieStylesResponseData, error) {
	return NewCookieStylesResponseData(Echo{"cookie": opts.Cookies}), nil
}

func TestStyles(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(echo{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	ctx := context.Background()
	tests := []struct {
		name string
		call func() (Echo, error)
		want Echo
	}{
		{
			name: "Every path style, with a value that needs escaping",
			call: func() (Echo, error) {
				return c.PathStyles(ctx, &PathStylesRequestOptions{PathParams: &PathStylesPathParams{Simple: "a b", Label: 5, Matrix: true, List: []string{"a", "b"}}})
			},
			want: Echo{"path": map[string]any{"simple": "a b", "label": 5.0, "matrix": true, "list": []any{"a", "b"}}},
		},
		{
			name: "Every query style",
			call: func() (Echo, error) {
				return c.QueryStyles(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{
					Form: []int{1, 2}, Csv: []string{"a", "b"}, Space: []string{"a", "b"}, Pipe: []string{"a", "b"},
					Deep: &Point{X: new(1), Y: new(2)}, Flat: &Point{X: new(3), Y: new(4)}, JSON: &Point{X: new(5)}, Needed: "yes",
				}})
			},
			want: Echo{"query": map[string]any{
				"form": []any{1.0, 2.0}, "csv": []any{"a", "b"}, "space": []any{"a", "b"}, "pipe": []any{"a", "b"},
				"deep": map[string]any{"x": 1.0, "y": 2.0}, "flat": map[string]any{"x": 3.0, "y": 4.0}, "json": map[string]any{"x": 5.0}, "needed": "yes",
			}},
		},
		{
			name: "A deep object with a list and an object inside",
			call: func() (Echo, error) {
				return c.QueryStyles(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{
					Filter: &Filter{Name: new("a"), Tags: []string{"b", "c"}, Size: &Point{X: new(1)}}, Needed: "yes",
				}})
			},
			want: Echo{"query": map[string]any{"filter": map[string]any{"name": "a", "tags": []any{"b", "c"}, "size": map[string]any{"x": 1.0}}, "needed": "yes"}},
		},
		{
			name: "An object whose list is unset",
			call: func() (Echo, error) {
				return c.QueryStyles(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{
					Filter: &Filter{Name: new("a"), Tags: []string{}}, Where: &Filter{Name: new("b")}, Needed: "yes",
				}})
			},
			want: Echo{"query": map[string]any{"filter": map[string]any{"name": "a"}, "where": map[string]any{"name": "b"}, "needed": "yes"}},
		},
		{
			name: "Query parameters left out stay out",
			call: func() (Echo, error) {
				return c.QueryStyles(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{Needed: "yes"}})
			},
			want: Echo{"query": map[string]any{"needed": "yes"}},
		},
		{
			name: "Header styles and formats",
			call: func() (Echo, error) {
				return c.HeaderStyles(ctx, &HeaderStylesRequestOptions{Headers: &HeaderStylesHeaders{
					XTags: []string{"a", "b"}, XPoint: &Point{X: new(1), Y: new(2)}, XWhen: new(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)),
				}})
			},
			want: Echo{"header": map[string]any{"X-Tags": []any{"a", "b"}, "X-Point": map[string]any{"x": 1.0, "y": 2.0}, "X-When": "2026-01-02T03:04:05Z"}},
		},
		{
			name: "Cookies",
			call: func() (Echo, error) {
				return c.CookieStyles(ctx, &CookieStylesRequestOptions{Cookies: &CookieStylesCookies{Session: new("abc"), Flags: []int{1, 2}}})
			},
			want: Echo{"cookie": map[string]any{"session": "abc", "flags": []any{1.0, 2.0}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.call()

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
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

	_, err = c.QueryStylesRequest(ctx, &QueryStylesRequestOptions{Query: &QueryStylesQuery{Where: &Filter{Tags: []string{"a"}}}})
	require.ErrorIs(t, err, runtime.ErrParamValue, "only a deep object can hold a list")
}
