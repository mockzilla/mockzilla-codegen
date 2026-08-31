// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// echo answers with what it received.
type echo struct{}

func (echo) PostJSON(_ context.Context, opts *PostJSONServiceRequestOptions) (*PostJSONResponseData, error) {
	return NewPostJSONResponseData(opts.Body), nil
}

func (echo) PostForm(_ context.Context, opts *PostFormServiceRequestOptions) (*PostFormResponseData, error) {
	return NewPostFormResponseData(opts.Body), nil
}

func (echo) Upload(_ context.Context, opts *UploadServiceRequestOptions) (*UploadResponseData, error) {
	content, err := opts.Body.File.Bytes()
	if err != nil {
		return nil, err
	}
	return NewUploadResponseData(UploadResponse200{
		"title": opts.Body.Title,
		"file":  string(content),
		"name":  opts.Body.File.Name(),
		"tags":  opts.Body.Tags,
		"meta":  opts.Body.Meta,
	}), nil
}

func (echo) PostText(_ context.Context, opts *PostTextServiceRequestOptions) (*PostTextResponseData, error) {
	if opts.BodyOctetStream != nil {
		return NewPostTextResponseData(new("bytes: " + string(opts.BodyOctetStream))), nil
	}
	if opts.BodyText == nil {
		return NewPostTextResponseData(new("nothing")), nil
	}
	return NewPostTextResponseData(new("text: " + *opts.BodyText)), nil
}

func (echo) PostAny(_ context.Context, opts *PostAnyServiceRequestOptions) (*PostAnyResponseData, error) {
	switch {
	case opts.BodyXML != nil:
		return NewPostAnyResponseData(new("xml: " + *opts.BodyXML)), nil
	case opts.BodyTextXML != nil:
		return NewPostAnyResponseData(new("text xml: " + *opts.BodyTextXML)), nil
	}
	return NewPostAnyResponseData(new("any: " + string(opts.BodyAny))), nil
}

func TestBodies(t *testing.T) {
	t.Parallel()

	router := NewRouter(echo{})
	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantBody    string
	}{
		{name: "JSON", path: "/json", body: `{"text":"hi","stars":3}`, contentType: "application/json", wantBody: `{"text":"hi","stars":3}`},
		{name: "JSON with a charset", path: "/json", body: `{"text":"hi"}`, contentType: "application/json; charset=utf-8", wantBody: `{"text":"hi"}`},
		{name: "An optional JSON body left empty", path: "/json", contentType: "application/json", wantBody: `null`},
		{name: "An optional body left out", path: "/json", wantBody: `null`},
		{name: "A form", path: "/form", body: `text=hi&stars=2`, contentType: "application/x-www-form-urlencoded", wantBody: `{"text":"hi","stars":2}`},
		{name: "A required form left empty", path: "/form", contentType: "application/x-www-form-urlencoded", wantStatus: 400, wantBody: `{"error":"invalid request body: request body is required"}`},
		{name: "Text", path: "/text", body: "hello", contentType: "text/plain", wantBody: `text: hello`},
		{name: "Bytes", path: "/text", body: "\x00\x01", contentType: "application/octet-stream", wantBody: "bytes: \x00\x01"},
		{name: "No text", path: "/text", wantBody: `nothing`},
		{name: "Two media types with one tag", path: "/any", body: "<a/>", contentType: "text/xml", wantBody: `text xml: <a/>`},
		{name: "The first of them", path: "/any", body: "<a/>", contentType: "application/xml", wantBody: `xml: <a/>`},
		{name: "A wildcard takes every other media type", path: "/any", body: "png", contentType: "image/png", wantBody: `any: png`},
		{name: "A media type the operation does not take", path: "/text", body: "{}", contentType: "application/json", wantStatus: 415, wantBody: `{"error":"invalid request body: unsupported content type: application/json"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
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

func TestMultipart(t *testing.T) {
	t.Parallel()

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
	req := httptest.NewRequest("POST", "/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	NewRouter(echo{}).ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.JSONEq(t, `{"title":"Cat","file":"meow","name":"cat.txt","tags":["a","b"],"meta":{"text":"inner"}}`, rec.Body.String())
}

func TestJSONDecoderOption(t *testing.T) {
	t.Parallel()

	strict := func(body io.Reader, dst any, isRequired bool) error {
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		return runtime.DecodeJSON(bytes.NewReader(bytes.ToUpper(data)), dst, isRequired)
	}
	req := httptest.NewRequest("POST", "/json", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewRouter(echo{}, WithJSONDecoder(strict)).ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, `{"text":"HI"}`, rec.Body.String())
}
