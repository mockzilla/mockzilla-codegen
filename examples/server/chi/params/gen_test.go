// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package params

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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

	router := NewRouter(echo{})
	tests := []struct {
		name       string
		path       string
		headers    http.Header
		wantStatus int
		wantBody   string
	}{
		{
			name:     "Every path style",
			path:     "/path/plain/.5/;matrix=true/a,b",
			wantBody: `{"path":{"simple":"plain","label":5,"matrix":true,"list":["a","b"]}}`,
		},
		{
			name:     "Every query style",
			path:     "/query?form=1&form=2&csv=a,b&space=a%20b&pipe=a|b&deep[x]=1&deep[y]=2&flat=x,3,y,4&json={\"x\":5}&needed=yes",
			wantBody: `{"query":{"form":[1,2],"csv":["a","b"],"space":["a","b"],"pipe":["a","b"],"deep":{"x":1,"y":2},"flat":{"x":3,"y":4},"json":{"x":5},"needed":"yes"}}`,
		},
		{
			name:     "Query parameters left out stay nil",
			path:     "/query?needed=yes",
			wantBody: `{"query":{"needed":"yes"}}`,
		},
		{
			name:       "A required query parameter left out",
			path:       "/query",
			wantStatus: 400,
			wantBody:   `{"error":"invalid query parameter \"needed\": parameter is required: needed"}`,
		},
		{
			name:       "A query parameter of the wrong type",
			path:       "/query?needed=yes&form=x",
			wantStatus: 400,
			wantBody:   `{"error":"invalid query parameter \"form\": invalid parameter value: \"x\" is no int"}`,
		},
		{
			name:     "Header styles and formats",
			path:     "/header",
			headers:  http.Header{"X-Tags": {"a,b"}, "X-Point": {"x,1,y,2"}, "X-When": {"2026-01-02T03:04:05Z"}},
			wantBody: `{"header":{"X-Tags":["a","b"],"X-Point":{"x":1,"y":2},"X-When":"2026-01-02T03:04:05Z"}}`,
		},
		{
			name:     "Cookies",
			path:     "/cookie",
			headers:  http.Header{"Cookie": {"session=abc; flags=1,2"}},
			wantBody: `{"cookie":{"session":"abc","flags":[1,2]}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("GET", tc.path, nil)
			for key, values := range tc.headers {
				req.Header[key] = values
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if tc.wantStatus == 0 {
				tc.wantStatus = 200
			}
			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}
