// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"context"
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

func (echo) PostAny(_ context.Context, opts *PostAnyServiceRequestOptions) (*PostAnyResponseData, error) {
	switch {
	case opts.BodyXML != nil:
		return NewPostAnyResponseData(new("xml: " + *opts.BodyXML)), nil
	case opts.BodyTextXML != nil:
		return NewPostAnyResponseData(new("text xml: " + *opts.BodyTextXML)), nil
	}
	return NewPostAnyResponseData(new("any: " + string(opts.BodyAny))), nil
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

func TestRequiredBody(t *testing.T) {
	t.Parallel()

	c := newClient(t)

	_, err := c.PostForm(context.Background(), &PostFormRequestOptions{})

	require.ErrorIs(t, err, runtime.ErrBodyEmpty)
}
