// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package ranges

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type service struct{}

func (service) AddAttachment(_ context.Context, opts *AddAttachmentServiceRequestOptions) (*AddAttachmentResponseData, error) {
	if opts.BodyText != nil {
		return NewAddAttachmentResponseData(new(len(*opts.BodyText))), nil
	}
	data, err := opts.BodyImage.Bytes()
	if err != nil {
		return nil, err
	}
	return NewAddAttachmentResponseData(new(len(data))), nil
}

func (service) PutScore(_ context.Context, opts *PutScoreServiceRequestOptions) (*PutScoreResponseData, error) {
	return NewPutScoreResponseData(new(*opts.Body * 2)), nil
}

func (service) AddCard(_ context.Context, opts *AddCardServiceRequestOptions) (*AddCardResponseData, error) {
	return NewAddCardResponseData(new(opts.Body.Name + " " + opts.Body.Photo.ContentType())), nil
}

func newClient(t *testing.T) (*Client, string) {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c, srv.URL
}

func post(t *testing.T, address, contentType, body string) int {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, address, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	return res.StatusCode
}

func TestAddAttachment(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)
	tests := []struct {
		name string
		opts *AddAttachmentRequestOptions
		want int
	}{
		{name: "Text goes under the text range", opts: &AddAttachmentRequestOptions{BodyText: new("hello")}, want: 5},
		{name: "An image goes under the image range", opts: &AddAttachmentRequestOptions{BodyImage: new(runtime.NewFile([]byte("PNG"), "a.png", "image/png"))}, want: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.AddAttachment(t.Context(), tc.opts)

			require.NoError(t, err)
			assert.Equal(t, new(tc.want), got)
		})
	}
}

func TestAddAttachmentTakesTheRangesOnly(t *testing.T) {
	t.Parallel()

	_, base := newClient(t)

	assert.Equal(t, http.StatusOK, post(t, base+"/attachments", "text/csv", "a,b"))
	assert.Equal(t, http.StatusUnsupportedMediaType, post(t, base+"/attachments", "application/json", `"hello"`))
}

func TestPutScore(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)
	got, err := c.PutScore(t.Context(), &PutScoreRequestOptions{Body: new(21)})

	require.NoError(t, err)
	assert.Equal(t, new(42), got)
}

func TestAddCard(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)
	got, err := c.AddCard(t.Context(), &AddCardRequestOptions{Body: &Card{Name: "Ada", Photo: new(runtime.NewFile([]byte("PNG"), "a.png", "image/png"))}})

	require.NoError(t, err)
	assert.Equal(t, new("Ada image/png"), got)
}
