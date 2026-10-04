// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		"type":  opts.Body.File.ContentType(),
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

func (echo) PutFile(_ context.Context, opts *PutFileServiceRequestOptions) (*PutFileResponseData, error) {
	return NewPutFileResponseData(opts.Body), nil
}

func (echo) PutXML(context.Context, *PutXMLServiceRequestOptions) (*PutXMLResponseData, error) {
	return NewPutXMLResponseData(), nil
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

func (echo) GetAnyText(context.Context, *GetAnyTextServiceRequestOptions) (*GetAnyTextResponseData, error) {
	return NewGetAnyTextResponseData(new(`{"type":"A+"}`)).WithHeaders(http.Header{"Content-Type": {"application/json"}}), nil
}

func (echo) GetAnyBytes(context.Context, *GetAnyBytesServiceRequestOptions) (*GetAnyBytesResponseData, error) {
	return NewGetAnyBytesResponseData([]byte(`{"type":"A+"}`)).WithHeaders(http.Header{"Content-Type": {"application/json"}}), nil
}

func newClient(t *testing.T) *Client {
	t.Helper()

	srv := httptest.NewServer(NewRouter(echo{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestBodies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name string
		call func() (any, error)
		want any
	}{
		{
			name: "JSON",
			call: func() (any, error) {
				return c.PostJSON(ctx, &PostJSONRequestOptions{Body: &Note{Text: "hi", Stars: new(3)}})
			},
			want: &Note{Text: "hi", Stars: new(3)},
		},
		{
			name: "An optional body left out",
			call: func() (any, error) { return c.PostJSON(ctx, nil) },
			want: (*Note)(nil),
		},
		{
			name: "A form",
			call: func() (any, error) {
				return c.PostForm(ctx, &PostFormRequestOptions{Body: &Note{Text: "hi", Stars: new(2)}})
			},
			want: &Note{Text: "hi", Stars: new(2)},
		},
		{
			name: "Text",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{BodyText: new("hello")}) },
			want: new("text: hello"),
		},
		{
			name: "Bytes",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{BodyOctetStream: []byte{0, 1}}) },
			want: new("bytes: \x00\x01"),
		},
		{
			name: "No body",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{}) },
			want: new("nothing"),
		},
		{
			name: "The first of two media types with one tag",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyXML: new("<a/>")}) },
			want: new("xml: <a/>"),
		},
		{
			name: "The second of them",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyTextXML: new("<a/>")}) },
			want: new("text xml: <a/>"),
		},
		{
			name: "A wildcard sends bytes as an octet stream",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyAny: []byte("png")}) },
			want: new("any: png"),
		},
		{
			name: "A wildcard string takes a JSON answer as it came",
			call: func() (any, error) { return c.GetAnyText(ctx, nil) },
			want: new(`{"type":"A+"}`),
		},
		{
			name: "Wildcard bytes take a JSON answer as it came",
			call: func() (any, error) { return c.GetAnyBytes(ctx, nil) },
			want: []byte(`{"type":"A+"}`),
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

func TestMultipart(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	body := &UploadRequestBody{
		Title: new("Cat"),
		File:  new(runtime.NewFile([]byte("meow"), "cat.txt", "text/plain")),
		Tags:  []string{"a", "b"},
		Meta:  &Note{Text: "inner"},
	}

	got, err := c.Upload(context.Background(), &UploadRequestOptions{Body: body})

	require.NoError(t, err)
	assert.Equal(t, UploadResponse200{
		"title": "Cat", "file": "meow", "name": "cat.txt", "type": "text/plain",
		"tags": []any{"a", "b"}, "meta": map[string]any{"text": "inner"},
	}, got)
}

func TestFile(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	png := runtime.NewFile([]byte("\x89PNG"), "cat.png", "image/png")

	got, err := c.PutFile(context.Background(), &PutFileRequestOptions{Body: &png})

	require.NoError(t, err)
	data, err := got.Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("\x89PNG"), data)
	assert.Equal(t, "image/png", got.ContentType())
}

func TestRequiredBody(t *testing.T) {
	t.Parallel()

	c := newClient(t)

	_, err := c.PostForm(context.Background(), &PostFormRequestOptions{})

	require.ErrorIs(t, err, runtime.ErrBodyEmpty)
}

func TestBodyTheClientCannotSend(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	tests := []struct {
		name string
		opts *PutXMLRequestOptions
		want error
	}{
		{name: "XML into a struct", opts: &PutXMLRequestOptions{Body: &Note{Text: "hi"}}, want: runtime.ErrContentType},
		{name: "No body", opts: &PutXMLRequestOptions{}, want: runtime.ErrBodyEmpty},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.PutXML(context.Background(), tc.opts)

			require.ErrorIs(t, err, tc.want)
		})
	}
}
