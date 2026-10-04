// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package servertest holds the behavior tests every server example runs, whatever its framework:
// the requests of each example spec and the responses a router of it must give.
package servertest

import (
	"bytes"
	"cmp"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Request is one request and the response it must get. Method is GET and WantStatus 200 when
// left out; WantHeaders are response headers to check.
type Request struct {
	Name        string
	Method      string
	Path        string
	Body        string
	ContentType string
	Headers     http.Header
	WantStatus  int
	WantBody    string
	WantHeaders map[string]string
}

// Basic are the requests of the basic example, in an order that creates a pet before it reads
// and deletes it. notFound is the body the framework answers an unknown path with.
func Basic(notFound string) []Request {
	return []Request{
		{Name: "Create a pet", Method: "POST", Path: "/pets", Body: `{"id":1,"name":"Rex"}`, ContentType: "application/json", WantStatus: 201, WantBody: `{"id":1,"name":"Rex"}`, WantHeaders: map[string]string{"Location": "/pets/1", "Content-Type": "application/json"}},
		{Name: "Get the pet", Path: "/pets/1", WantBody: `{"id":1,"name":"Rex"}`},
		{Name: "List with a limit", Path: "/pets?limit=0", WantBody: `[]`},
		{Name: "No such pet", Path: "/pets/2", WantStatus: 404},
		{Name: "A path parameter that is not a number", Path: "/pets/x", WantStatus: 400, WantBody: `{"error":"invalid path parameter \"id\": invalid parameter value: \"x\" is no int"}`},
		{Name: "An error as text when the client asks for it", Path: "/pets/x", Headers: http.Header{"Accept": {"text/plain"}}, WantStatus: 400, WantBody: `invalid path parameter "id": invalid parameter value: "x" is no int`, WantHeaders: map[string]string{"Content-Type": "text/plain; charset=utf-8"}},
		{Name: "A body in another media type", Method: "POST", Path: "/pets", Body: `<pet/>`, ContentType: "application/xml", WantStatus: 415, WantBody: `{"error":"invalid request body: unsupported content type: application/xml"}`},
		{Name: "A required body left out", Method: "POST", Path: "/pets", WantStatus: 400, WantBody: `{"error":"invalid request body: request body is required"}`},
		{Name: "A body that is not JSON", Method: "POST", Path: "/pets", Body: `{`, ContentType: "application/json", WantStatus: 400, WantBody: `{"error":"invalid request body: unexpected end of JSON input"}`},
		{Name: "The service fails", Method: "POST", Path: "/pets", Body: `{"id":2,"name":"boom"}`, ContentType: "application/json", WantStatus: 500, WantBody: `{"error":"internal server error"}`},
		{Name: "Delete the pet", Method: "DELETE", Path: "/pets/1", WantStatus: 204},
		{Name: "Text response", Path: "/ping", WantBody: `pong`},
		{Name: "Unknown route", Path: "/nope", WantStatus: 404, WantBody: notFound},
	}
}

// Bodies are the requests of the bodies example, each body decoded by its media type.
var Bodies = []Request{
	{Name: "JSON", Method: "POST", Path: "/json", Body: `{"text":"hi","stars":3}`, ContentType: "application/json", WantBody: `{"text":"hi","stars":3}`},
	{Name: "JSON with a charset", Method: "POST", Path: "/json", Body: `{"text":"hi"}`, ContentType: "application/json; charset=utf-8", WantBody: `{"text":"hi"}`},
	{Name: "An optional JSON body left empty", Method: "POST", Path: "/json", ContentType: "application/json", WantBody: `null`},
	{Name: "An optional body left out", Method: "POST", Path: "/json", WantBody: `null`},
	{Name: "A form", Method: "POST", Path: "/form", Body: `text=hi&stars=2`, ContentType: "application/x-www-form-urlencoded", WantBody: `{"text":"hi","stars":2}`},
	{Name: "A required form left empty", Method: "POST", Path: "/form", ContentType: "application/x-www-form-urlencoded", WantStatus: 400, WantBody: `{"error":"invalid request body: request body is required"}`},
	{Name: "Text", Method: "POST", Path: "/text", Body: "hello", ContentType: "text/plain", WantBody: `text: hello`},
	{Name: "Bytes", Method: "POST", Path: "/text", Body: "\x00\x01", ContentType: "application/octet-stream", WantBody: "bytes: \x00\x01"},
	{Name: "No text", Method: "POST", Path: "/text", WantBody: `nothing`},
	{Name: "A file streams in and out", Method: "PUT", Path: "/file", Body: "\x89PNG", ContentType: "image/png", WantBody: "\x89PNG", WantHeaders: map[string]string{"Content-Type": "image/png"}},
	{Name: "A required file left empty", Method: "PUT", Path: "/file", ContentType: "image/png", WantStatus: 400, WantBody: `{"error":"invalid request body: request body is required"}`},
	{Name: "Two media types with one tag", Method: "POST", Path: "/any", Body: "<a/>", ContentType: "text/xml", WantBody: `text xml: <a/>`},
	{Name: "The first of them", Method: "POST", Path: "/any", Body: "<a/>", ContentType: "application/xml", WantBody: `xml: <a/>`},
	{Name: "A wildcard takes every other media type", Method: "POST", Path: "/any", Body: "png", ContentType: "image/png", WantBody: `any: png`},
	{Name: "A media type the operation does not take", Method: "POST", Path: "/text", Body: "{}", ContentType: "application/json", WantStatus: 415, WantBody: `{"error":"invalid request body: unsupported content type: application/json"}`},
	{Name: "A form answer", Path: "/form", WantBody: `stars=2&text=hi`, WantHeaders: map[string]string{"Content-Type": "application/x-www-form-urlencoded"}},
	{Name: "A string answered as JSON", Path: "/quote", WantBody: `"hi"`, WantHeaders: map[string]string{"Content-Type": "application/json"}},
	{Name: "A number answered as text", Path: "/count", WantBody: `3`, WantHeaders: map[string]string{"Content-Type": "text/plain"}},
	{Name: "One event per frame", Path: "/notes", WantBody: "data: {\"text\":\"a\"}\n\ndata: {\"text\":\"b\",\"stars\":1}\n\n", WantHeaders: map[string]string{"Content-Type": "text/event-stream"}},
}

// Errors are the requests of the errors example: requests and responses that fail validation, and
// the error types of the spec answered with their status.
var Errors = []Request{
	{Name: "A pet", Path: "/pets/1", WantBody: `{"name":"Rex","age":3}`},
	{Name: "The request fails validation", Path: "/pets/0", WantStatus: 400, WantBody: `{"error":"invalid request: path.id: must be at least 1"}`},
	{Name: "A query value outside the enum", Path: "/pets/1?fields=color", WantStatus: 400, WantBody: `{"error":"invalid request: query.fields[0]: must be one of name, age"}`},
	{Name: "The response fails validation", Path: "/pets/2", WantStatus: 500, WantBody: `{"error":"invalid response: name: must be at least 1 characters long"}`},
	{Name: "A typed error carries the status and media type of its response", Path: "/pets/9", WantStatus: 404, WantBody: `{"detail":"no pet 9"}`, WantHeaders: map[string]string{"Content-Type": "application/problem+json"}},
	{Name: "A wrapped typed error", Path: "/pets/4", WantStatus: 404, WantBody: `{"detail":"wrapped"}`},
	{Name: "A typed error behind a pointer", Path: "/pets/5", WantStatus: 404, WantBody: `{"detail":"pointer"}`},
	{Name: "A response type that is not an error is a response", Path: "/pets/3", WantStatus: 409, WantBody: `{"detail":"locked","until":"later"}`},
	{Name: "A body that fails validation", Method: "PUT", Path: "/pets/1", Body: `{"name":""}`, ContentType: "application/json", WantStatus: 400, WantBody: `{"error":"invalid request: body.name: must be at least 1 characters long"}`},
	{Name: "A read-only field is checked in the response only", Method: "PUT", Path: "/pets/1", Body: `{"name":"Rex","age":-1}`, ContentType: "application/json", WantStatus: 500, WantBody: `{"error":"invalid response: age: must be at least 0"}`},
	{Name: "A valid body", Method: "PUT", Path: "/pets/1", Body: `{"name":"Rex"}`, ContentType: "application/json", WantBody: `{"name":"Rex"}`},
}

// Params are the requests of the params example: every parameter style of each location.
var Params = []Request{
	{
		Name:     "Every path style",
		Path:     "/path/plain/.5/;matrix=true/a,b",
		WantBody: `{"path":{"simple":"plain","label":5,"matrix":true,"list":["a","b"]}}`,
	},
	{
		Name:     "Every query style",
		Path:     "/query?form=1&form=2&csv=a,b&space=a%20b&pipe=a|b&deep[x]=1&deep[y]=2&flat=x,3,y,4&json={\"x\":5}&id=7&needed=yes&limit=5",
		WantBody: `{"query":{"form":[1,2],"csv":["a","b"],"space":["a","b"],"pipe":["a","b"],"deep":{"x":1,"y":2},"flat":{"x":3,"y":4},"json":{"x":5},"id":7,"needed":"yes","limit":5}}`,
	},
	{
		Name:     "A union query parameter that is no number is a string",
		Path:     "/query?id=a7&needed=yes",
		WantBody: `{"query":{"id":"a7","needed":"yes","limit":20}}`,
	},
	{
		Name:     "Query parameters left out stay nil or take their default",
		Path:     "/query?needed=yes",
		WantBody: `{"query":{"needed":"yes","limit":20}}`,
	},
	{
		Name:       "A required query parameter left out",
		Path:       "/query",
		WantStatus: 400,
		WantBody:   `{"error":"invalid query parameter \"needed\": parameter is required: needed"}`,
	},
	{
		Name:       "A query parameter of the wrong type",
		Path:       "/query?needed=yes&form=x",
		WantStatus: 400,
		WantBody:   `{"error":"invalid query parameter \"form\": invalid parameter value: \"x\" is no int"}`,
	},
	{
		Name:     "Header styles and formats",
		Path:     "/header",
		Headers:  http.Header{"X-Tags": {"a,b"}, "X-Point": {"x,1,y,2"}, "X-When": {"2026-01-02T03:04:05Z"}, "X-Limit": {"true"}},
		WantBody: `{"header":{"X-Tags":["a","b"],"X-Point":{"x":1,"y":2},"X-When":"2026-01-02T03:04:05Z","X-Limit":true}}`,
	},
	{
		Name:       "A union header that no variant takes",
		Path:       "/header",
		Headers:    http.Header{"X-Limit": {"many"}},
		WantStatus: 400,
		WantBody:   `{"error":"invalid header parameter \"X-Limit\": no union variant matches for a JSON string"}`,
	},
	{
		Name:     "Cookies",
		Path:     "/cookie",
		Headers:  http.Header{"Cookie": {"session=abc; flags=1,2"}},
		WantBody: `{"cookie":{"session":"abc","flags":[1,2]}}`,
	},
	{
		Name:     "A querystring parameter reads the whole query",
		Path:     "/search?name=rex&tag=a&tag=b",
		WantBody: `{"search":{"name":"rex","tag":["a","b"]}}`,
	},
	{
		Name:     "A querystring parameter left out stays nil",
		Path:     "/search",
		WantBody: `{"search":null}`,
	},
}

// Run sends every request to h, in order, and checks its response.
func Run(t *testing.T, h http.Handler, requests []Request) {
	t.Helper()

	for _, tc := range requests {
		t.Run(tc.Name, func(t *testing.T) {
			req := httptest.NewRequest(cmp.Or(tc.Method, http.MethodGet), tc.Path, strings.NewReader(tc.Body))
			if tc.ContentType != "" {
				req.Header.Set("Content-Type", tc.ContentType)
			}
			for key, values := range tc.Headers {
				req.Header[key] = values
			}
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			assert.Equal(t, cmp.Or(tc.WantStatus, http.StatusOK), rec.Code)
			assert.Equal(t, tc.WantBody, rec.Body.String())
			for key, want := range tc.WantHeaders {
				assert.Equal(t, want, rec.Header().Get(key), key)
			}
		})
	}
}

// Multipart uploads a form with fields, a file and a JSON part to /upload of the bodies example
// and checks that h echoes every part.
func Multipart(t *testing.T, h http.Handler) {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("title", "Cat"))
	require.NoError(t, mw.WriteField("tags", "a"))
	require.NoError(t, mw.WriteField("tags", "b"))
	require.NoError(t, mw.WriteField("meta", `{"text":"inner"}`))
	part, err := mw.CreateFormFile("file", "cat.txt")
	require.NoError(t, err)
	_, err = io.WriteString(part, "meow")
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"title":"Cat","file":"meow","name":"cat.txt","tags":["a","b"],"meta":{"text":"inner"}}`, rec.Body.String())
}
