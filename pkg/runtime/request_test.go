// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"cmp"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func parseURL(t *testing.T, s string) *url.URL {
	t.Helper()

	u, err := url.Parse(s)
	require.NoError(t, err)
	return u
}

func TestRequestBuilder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		base        string
		build       func(b *RequestBuilder)
		wantURL     string
		wantHeader  http.Header
		wantBody    string
		wantLength  int64
		wantErr     error
		wantErrText string
	}{
		{
			name: "Path parameters of every style, escaped but for the delimiters",
			path: "/pets/{id}/{tags}/{rgb}",
			build: func(b *RequestBuilder) {
				b.PathParam("a b/c", Param{Name: "id", Style: StyleSimple})
				b.PathParam(colors, Param{Name: "tags", Style: StyleLabel, IsExplode: true})
				b.PathParam(color, Param{Name: "rgb", Style: StyleMatrix, IsExplode: true})
			},
			wantURL: "http://api.test/v1/pets/a%20b%2Fc/.blue.black.brown/;R=100;G=200;B=150",
		},
		{
			name: "Query, header and cookie parameters",
			build: func(b *RequestBuilder) {
				b.QueryParam(colors, Param{Name: "color", Style: StyleForm, IsExplode: true})
				b.QueryParam(color, Param{Name: "rgb", Style: StyleDeepObject})
				b.QueryParam(nil, Param{Name: "none", Style: StyleForm})
				b.HeaderParam(colors, Param{Name: "X-Colors", Style: StyleSimple})
				b.HeaderParam((*string)(nil), Param{Name: "X-None", Style: StyleSimple})
				b.CookieParam("abc", Param{Name: "session", Style: StyleForm})
				b.CookieParam(colors, Param{Name: "flags", Style: StyleForm})
			},
			wantURL:    "http://api.test/v1/pets?color=blue&color=black&color=brown&rgb%5BB%5D=150&rgb%5BG%5D=200&rgb%5BR%5D=100",
			wantHeader: http.Header{"X-Colors": {"blue,black,brown"}, "Cookie": {"session=abc; flags=\"blue,black,brown\""}},
		},
		{
			name: "A required query parameter that is nil",
			build: func(b *RequestBuilder) {
				b.QueryParam(nil, Param{Name: "needed", IsRequired: true})
			},
			wantErr: ErrParamMissing,
		},
		{
			name: "A nil path parameter",
			build: func(b *RequestBuilder) {
				b.PathParam((*int)(nil), Param{Name: "id"})
			},
			wantErrText: "parameter is required: id",
		},
		{
			name: "A path parameter that cannot be written",
			build: func(b *RequestBuilder) {
				b.PathParam(make(chan int), Param{Name: "id"})
			},
			wantErr: ErrParamValue,
		},
		{
			name: "A header parameter that cannot be written",
			build: func(b *RequestBuilder) {
				b.HeaderParam(make(chan int), Param{Name: "X-Bad"})
			},
			wantErr: ErrParamValue,
		},
		{
			name: "A cookie parameter that cannot be written",
			build: func(b *RequestBuilder) {
				b.CookieParam(make(chan int), Param{Name: "bad"})
			},
			wantErr: ErrParamValue,
		},
		{
			name: "A query parameter that cannot be written",
			build: func(b *RequestBuilder) {
				b.QueryParam(make(chan int), Param{Name: "bad"})
			},
			wantErr: ErrParamValue,
		},
		{
			name: "The first error stops the rest",
			build: func(b *RequestBuilder) {
				b.PathParam(make(chan int), Param{Name: "id"})
				b.PathParam(1, Param{Name: "tags"})
				b.QueryParam("x", Param{Name: "q"})
				b.HeaderParam("x", Param{Name: "X-H"})
				b.CookieParam("x", Param{Name: "c"})
				b.JSONBody(1, "application/json")
				b.FormBody(color)
				b.MultipartBody(color)
				b.FileBody(NewFile(nil, "a", ""), "")
				b.TextBody("x", "text/plain")
			},
			wantErr: ErrParamValue,
		},
		{
			name: "A placeholder nothing filled",
			path: "/pets/{id}/{tags}",
			build: func(b *RequestBuilder) {
				b.PathParam(1, Param{Name: "id"})
			},
			wantErrText: "parameter is required: tags",
		},
		{
			name:       "A JSON body",
			build:      func(b *RequestBuilder) { b.JSONBody(color, "application/vnd.color+json") },
			wantHeader: http.Header{"Content-Type": {"application/vnd.color+json"}},
			wantBody:   `{"R":100,"G":200,"B":150}`,
			wantLength: 25,
		},
		{
			name:        "A JSON body that cannot be written",
			build:       func(b *RequestBuilder) { b.JSONBody(make(chan int), "application/json") },
			wantErrText: "json: unsupported type: chan int",
		},
		{
			name:       "A form body",
			build:      func(b *RequestBuilder) { b.FormBody(color) },
			wantHeader: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
			wantBody:   "B=150&G=200&R=100",
			wantLength: 17,
		},
		{
			name:    "A form body that is no object",
			build:   func(b *RequestBuilder) { b.FormBody("text") },
			wantErr: ErrBodyValue,
		},
		{
			name:    "A multipart body that is no struct",
			build:   func(b *RequestBuilder) { b.MultipartBody("text") },
			wantErr: ErrBodyValue,
		},
		{
			name:       "A text body",
			build:      func(b *RequestBuilder) { b.TextBody("hello", "text/plain") },
			wantHeader: http.Header{"Content-Type": {"text/plain"}},
			wantBody:   "hello",
			wantLength: 5,
		},
		{
			name:       "A bytes body",
			build:      func(b *RequestBuilder) { b.BytesBody([]byte{0, 1}, "application/octet-stream") },
			wantHeader: http.Header{"Content-Type": {"application/octet-stream"}},
			wantBody:   "\x00\x01",
			wantLength: 2,
		},
		{
			name:       "A file body under its own content type, with its size",
			build:      func(b *RequestBuilder) { b.FileBody(NewFile([]byte("meow"), "cat.txt", "text/plain"), "") },
			wantHeader: http.Header{"Content-Type": {"text/plain"}},
			wantBody:   "meow",
			wantLength: 4,
		},
		{
			name: "A file body under the media type of the operation",
			build: func(b *RequestBuilder) {
				b.FileBody(NewFileReader(strings.NewReader("meow"), "cat.txt", "", -1), "image/png")
			},
			wantHeader: http.Header{"Content-Type": {"image/png"}},
			wantBody:   "meow",
		},
		{
			name:        "A file body that cannot be opened",
			build:       func(b *RequestBuilder) { b.FileBody(NewFileFromMultipart(&multipart.FileHeader{Filename: "gone"}), "") },
			wantErrText: "open : no such file or directory",
		},
		{
			name:        "A base path with a trailing slash and a path that does not unescape",
			base:        "http://api.test/v1/",
			build:       func(b *RequestBuilder) { b.path = "/%zz" },
			wantErrText: "invalid parameter value: invalid URL escape \"%zz\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b := NewRequestBuilder(http.MethodPost, cmp.Or(tc.path, "/pets"))
			tc.build(b)

			req, err := b.Build(context.Background(), parseURL(t, cmp.Or(tc.base, "http://api.test/v1")))

			switch {
			case tc.wantErrText != "":
				require.EqualError(t, err, tc.wantErrText)
				return
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, req.Method)
			if tc.wantURL != "" {
				assert.Equal(t, tc.wantURL, req.URL.String())
			}
			for key, values := range tc.wantHeader {
				assert.Equal(t, values, req.Header[key], key)
			}
			if tc.wantBody != "" {
				body, readErr := io.ReadAll(req.Body)
				require.NoError(t, readErr)
				assert.Equal(t, tc.wantBody, string(body))
				assert.Equal(t, tc.wantLength, req.ContentLength)
			}
		})
	}
}

func TestRequestBuilderQueryAndHeaderInOnePlace(t *testing.T) {
	t.Parallel()

	b := NewRequestBuilder(http.MethodGet, "/search")
	b.QueryParam("a b", Param{Name: "q", Style: StyleForm})
	b.QueryParam(true, Param{Name: "exact", Style: StyleForm})
	b.HeaderParam(color, Param{Name: "X-Point", Style: StyleSimple, IsExplode: true})

	req, err := b.Build(context.Background(), parseURL(t, "https://api.test"))

	require.NoError(t, err)
	assert.Equal(t, "https://api.test/search?exact=true&q=a+b", req.URL.String())
	assert.Equal(t, "R=100,G=200,B=150", req.Header.Get("X-Point"))
	assert.Nil(t, req.Body)
}

func TestRequestBuilderKeepsAnEscapedBasePath(t *testing.T) {
	t.Parallel()

	b := NewRequestBuilder(http.MethodGet, "/pets/{id}")
	b.PathParam("a/b", Param{Name: "id"})

	req, err := b.Build(context.Background(), parseURL(t, "http://api.test/v%201"))

	require.NoError(t, err)
	assert.Equal(t, "http://api.test/v%201/pets/a%2Fb", req.URL.String())
	assert.Equal(t, "/v 1/pets/a/b", req.URL.Path)
}

func TestRequestBuilderRejectsAMethodTheRequestCannotTake(t *testing.T) {
	t.Parallel()

	b := NewRequestBuilder("bad method", "/")

	_, err := b.Build(context.Background(), parseURL(t, "http://api.test"))

	require.EqualError(t, err, `net/http: invalid method "bad method"`)
}

func TestParseBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "A URL with a scheme and a host", raw: "https://api.test/v1", want: "https://api.test/v1"},
		{name: "A path alone", raw: "/v1", wantErr: `invalid base URL: "/v1" needs a scheme and a host`},
		{name: "A host without a scheme", raw: "api.test", wantErr: `invalid base URL: "api.test" needs a scheme and a host`},
		{name: "No URL at all", raw: "http://a b", wantErr: `invalid base URL: parse "http://a b": invalid character " " in host name`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			u, err := ParseBaseURL(tc.raw)

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				require.ErrorIs(t, err, ErrBaseURL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, u.String())
		})
	}
}

func TestSend(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://api.test", nil)
	require.NoError(t, err)
	failing := errors.New("no route to host")
	tests := []struct {
		name     string
		doer     Doer
		wantBody string
		wantErr  error
	}{
		{
			name: "The body is read and can be read again",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("pong"))}, nil
			}),
			wantBody: "pong",
		},
		{
			name: "A response without a body",
			doer: doerFunc(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 204}, nil }),
		},
		{
			name:    "The request fails",
			doer:    doerFunc(func(*http.Request) (*http.Response, error) { return nil, failing }),
			wantErr: failing,
		},
		{
			name: "The body fails to read",
			doer: doerFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(errReader{})}, nil
			}),
			wantErr: io.ErrUnexpectedEOF,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, body, sendErr := Send(tc.doer, req)

			if tc.wantErr != nil {
				require.ErrorIs(t, sendErr, tc.wantErr)
				return
			}
			require.NoError(t, sendErr)
			assert.Equal(t, tc.wantBody, string(body))
			if res.Body != nil {
				again, readErr := io.ReadAll(res.Body)
				require.NoError(t, readErr)
				assert.Equal(t, tc.wantBody, string(again))
			}
		})
	}
}

func TestIsNil(t *testing.T) {
	t.Parallel()

	var m map[string]int
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "No value", want: true},
		{name: "A nil pointer", value: (*int)(nil), want: true},
		{name: "A nil slice", value: []string(nil), want: true},
		{name: "A nil map", value: m, want: true},
		{name: "A value", value: 0},
		{name: "An empty slice", value: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isNil(tc.value))
		})
	}
}
