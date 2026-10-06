// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package checked

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// service answers with the body as it got it.
type service struct{}

func (service) AddPet(_ context.Context, opts *AddPetServiceRequestOptions) (*AddPetResponseData, error) {
	return NewAddPetResponseData(cmp.Or(opts.BodyJSON, opts.BodyForm)), nil
}

func (service) Upload(_ context.Context, opts *UploadServiceRequestOptions) (*UploadResponseData, error) {
	return NewUploadResponseData(new(*opts.Body.Note + " " + opts.Body.Photo.Name())), nil
}

func TestAddPet(t *testing.T) {
	t.Parallel()

	const (
		typeJSON = "application/json"
		typeForm = "application/x-www-form-urlencoded"
	)
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		want        string
	}{
		{
			name: "Missing properties get their defaults", contentType: typeJSON, wantStatus: http.StatusOK,
			body: `{"name":"Rex","owner":{"id":1},"toys":[{"name":"ball"}]}`,
			want: `{"name":"Rex","age":1,"size":"m","owner":{"id":1,"city":"Berlin"},"toys":[{"name":"ball","color":"red"}]}`,
		},
		{
			name: "A sent null stays null", contentType: typeJSON, wantStatus: http.StatusOK,
			body: `{"name":"Rex","tag":null,"size":null,"owner":{"id":1}}`,
			want: `{"name":"Rex","age":1,"owner":{"id":1,"city":"Berlin"}}`,
		},
		{
			name: "A present empty value passes", contentType: typeJSON, wantStatus: http.StatusOK,
			body: `{"name":"","owner":{"id":0}}`,
			want: `{"name":"","age":1,"size":"m","owner":{"id":0,"city":"Berlin"}}`,
		},
		{
			name: "An empty list stays empty", contentType: typeJSON, wantStatus: http.StatusOK,
			body: `{"name":"Rex","owner":{"id":1},"tags":[],"toys":[]}`,
			want: `{"name":"Rex","age":1,"size":"m","owner":{"id":1,"city":"Berlin"},"tags":[],"toys":[]}`,
		},
		{
			name: "A missing required key is an error", contentType: typeJSON, wantStatus: http.StatusBadRequest,
			body: `{"owner":{}}`,
			want: `{"error":"invalid request: body.name: is required; body.owner.id: is required"}`,
		},
		{
			name: "A null the spec does not allow is an error", contentType: typeJSON, wantStatus: http.StatusBadRequest,
			body: `{"name":null,"owner":{"id":1},"tags":["a",null]}`,
			want: `{"error":"invalid request: body.name: must not be null; body.tags[1]: must not be null"}`,
		},
		{
			name: "A null body is an error", contentType: typeJSON, wantStatus: http.StatusBadRequest,
			body: `null`,
			want: `{"error":"invalid request: body: must not be null"}`,
		},
		{
			name: "An unknown key is an error", contentType: typeJSON, wantStatus: http.StatusBadRequest,
			body: `{"name":"Rex","owner":{"id":1},"x":1}`,
			want: `{"error":"invalid request: body.x: is not allowed"}`,
		},
		{
			name: "A form gets its defaults", contentType: typeForm, wantStatus: http.StatusOK,
			body: "name=Rex&owner[id]=1&toys[0][name]=ball",
			want: `{"name":"Rex","age":1,"size":"m","owner":{"id":1,"city":"Berlin"},"toys":[{"name":"ball","color":"red"}]}`,
		},
		{
			name: "A form without a required field is an error", contentType: typeForm, wantStatus: http.StatusBadRequest,
			body: "owner[id]=1",
			want: `{"error":"invalid request: body.name: is required"}`,
		},
		{
			name: "A form with an unknown field is an error", contentType: typeForm, wantStatus: http.StatusBadRequest,
			body: "name=Rex&owner[id]=1&x=1",
			want: `{"error":"invalid request: body.x: is not allowed"}`,
		},
	}

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := http.Post(srv.URL+"/pets", tc.contentType, strings.NewReader(tc.body))
			require.NoError(t, err)
			defer res.Body.Close()
			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatus, res.StatusCode)
			assert.JSONEq(t, tc.want, string(data))
		})
	}
}

func TestUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		isPhoto    bool
		note       string
		wantStatus int
		want       string
	}{
		{name: "A missing field gets its default", isPhoto: true, wantStatus: http.StatusOK, want: "none rex.png"},
		{name: "A sent field keeps its value", isPhoto: true, note: "hi", wantStatus: http.StatusOK, want: "hi rex.png"},
		{name: "A missing required file is an error", note: "hi", wantStatus: http.StatusBadRequest, want: "invalid request: body.photo: is required"},
	}

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			if tc.note != "" {
				require.NoError(t, mw.WriteField("note", tc.note))
			}
			if tc.isPhoto {
				fw, err := mw.CreateFormFile("photo", "rex.png")
				require.NoError(t, err)
				_, err = fw.Write([]byte("png"))
				require.NoError(t, err)
			}
			require.NoError(t, mw.Close())

			req, err := http.NewRequest(http.MethodPost, srv.URL+"/uploads", &buf)
			require.NoError(t, err)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("Accept", "text/plain")
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()
			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, tc.wantStatus, res.StatusCode)
			assert.Equal(t, tc.want, string(data))
		})
	}
}

func TestWithPresence(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(service{}, WithPresence(runtime.Presence{})))
	t.Cleanup(srv.Close)
	res, err := http.Post(srv.URL+"/pets", "application/json", strings.NewReader(`{"name":"Rex","owner":{"id":1},"x":1}`))
	require.NoError(t, err)
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, res.StatusCode, "an empty table checks nothing")
	assert.JSONEq(t, `{"name":"Rex","owner":{"id":1}}`, string(data), "and sets no default")
}
