// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package server

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// service echoes the shape it read, and fails with a Fault for the name busy.
type service struct{}

func (service) PostForm(_ context.Context, opts *PostFormServiceRequestOptions) (*PostFormResponseData, error) {
	if opts.Body.Name != nil && *opts.Body.Name == "busy" {
		return nil, Fault{Busy: &Busy{Message: "busy", Retry: 5}}
	}
	return NewPostFormResponseData200(opts.Body), nil
}

func (service) PostMultipart(_ context.Context, opts *PostMultipartServiceRequestOptions) (*PostMultipartResponseData, error) {
	return NewPostMultipartResponseData(opts.Body), nil
}

// PostAttachment says which variant it read: the link, or the file with its checksum.
func (service) PostAttachment(_ context.Context, opts *PostAttachmentServiceRequestOptions) (*PostAttachmentResponseData, error) {
	a := cmp.Or(opts.BodyForm, opts.BodyMultipart)
	if a.Link != nil {
		return NewPostAttachmentResponseData(new("link " + a.Link.URL)), nil
	}
	data, err := a.Upload.File.Bytes()
	if err != nil {
		return nil, err
	}
	return NewPostAttachmentResponseData(new(fmt.Sprintf("file %s %s %x", a.Upload.File.Name(), data, a.Upload.Checksum))), nil
}

func newClient(t *testing.T) *Client {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestUnionFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	labels := ShapeLabels{Main: new("a")}
	labels.Set("extra", "b")
	tests := []struct {
		name  string
		shape Shape
	}{
		{name: "An object variant", shape: Shape{Vertex: &Vertex{Point: &Point{X: 1, Y: 2}}, Labels: &labels, Origin: &Point{X: 3, Y: 4}, Stamp: []byte{0, 255}}},
		{name: "A string variant", shape: Shape{Name: new("n"), Vertex: &Vertex{String: new("top")}}},
	}

	c := newClient(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.PostForm(t.Context(), &PostFormRequestOptions{Body: &tc.shape})
			require.NoError(t, err)
			assert.Equal(t, tc.shape, *got)

			got, err = c.PostMultipart(t.Context(), &PostMultipartRequestOptions{Body: &tc.shape})
			require.NoError(t, err)
			assert.Equal(t, tc.shape, *got)
		})
	}
}

func TestUnionFieldsFromText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "Plain text is the string variant", value: "abc", want: `{"vertex":"abc"}`},
		{name: "A number no variant takes is the string", value: "12", want: `{"vertex":"12"}`},
		{name: "JSON is the object variant", value: `{"x":1,"y":2}`, want: `{"vertex":{"x":1,"y":2}}`},
	}

	h := NewRouter(service{})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			form := url.Values{"vertex": {tc.value}}
			assert.JSONEq(t, tc.want, post(t, h, "/form", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
			body, contentType := multipartForm(t, form)
			assert.JSONEq(t, tc.want, post(t, h, "/multipart", contentType, body))
		})
	}
}

func TestUnionFieldsFromKeys(t *testing.T) {
	t.Parallel()

	form := url.Values{"vertex[x]": {"1"}, "vertex[y]": {"2"}, "labels[main]": {"a"}, "labels[extra]": {"b"}, "stamp": {"AP8="}}
	want := `{"vertex":{"x":1,"y":2},"labels":{"main":"a","extra":"b"},"stamp":"AP8="}`

	h := NewRouter(service{})
	assert.JSONEq(t, want, post(t, h, "/form", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
	body, contentType := multipartForm(t, form)
	assert.JSONEq(t, want, post(t, h, "/multipart", contentType, body))
}

func TestUnionBodies(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	link := &Attachment{Link: &Link{URL: "https://example.com/a.png"}}
	upload := &Attachment{Upload: &Upload{File: runtime.NewFile([]byte("PNG"), "a.png", "image/png"), Checksum: []byte{0xca, 0xfe}}}

	got, err := c.PostAttachment(t.Context(), &PostAttachmentRequestOptions{BodyForm: link})
	require.NoError(t, err)
	assert.Equal(t, "link https://example.com/a.png", *got)

	got, err = c.PostAttachment(t.Context(), &PostAttachmentRequestOptions{BodyMultipart: link})
	require.NoError(t, err)
	assert.Equal(t, "link https://example.com/a.png", *got)

	got, err = c.PostAttachment(t.Context(), &PostAttachmentRequestOptions{BodyMultipart: upload})
	require.NoError(t, err)
	assert.Equal(t, "file a.png PNG cafe", *got)
}

func TestUnionError(t *testing.T) {
	t.Parallel()

	_, err := newClient(t).PostForm(t.Context(), &PostFormRequestOptions{Body: &Shape{Name: new("busy")}})

	var fault *Fault
	require.ErrorAs(t, err, &fault)
	assert.Equal(t, Fault{Busy: &Busy{Message: "busy", Retry: 5}}, *fault)
	assert.Equal(t, "busy", fault.Error())
}

// multipartForm writes values as text parts.
func multipartForm(t *testing.T, values url.Values) (io.Reader, string) {
	t.Helper()

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for name, items := range values {
		for _, item := range items {
			require.NoError(t, mw.WriteField(name, item))
		}
	}
	require.NoError(t, mw.Close())
	return &b, mw.FormDataContentType()
}

func post(t *testing.T, h http.Handler, path, contentType string, body io.Reader) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return rec.Body.String()
}
