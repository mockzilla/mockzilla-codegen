// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package responses

import (
	"context"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type service struct{}

func (service) Ping(context.Context, *PingServiceRequestOptions) (*PingResponseData, error) {
	return NewPingResponseData("pong"), nil
}

func (service) GetMotd(context.Context, *GetMotdServiceRequestOptions) (*GetMotdResponseData, error) {
	return NewGetMotdResponseData(new("hello")), nil
}

func (service) GetProfile(context.Context, *GetProfileServiceRequestOptions) (*GetProfileResponseData, error) {
	return NewGetProfileResponseData(&Profile{Name: "Ada", Avatar: new(runtime.NewFile([]byte("PNG"), "a.png", "image/png"))}), nil
}

func newClient(t *testing.T) (*Client, string) {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c, srv.URL
}

func mediaType(t *testing.T, address string) string {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())

	mt, _, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	require.NoError(t, err)
	return mt
}

func TestPing(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)
	got, err := c.Ping(t.Context(), nil)

	require.NoError(t, err)
	assert.Equal(t, "pong", got)
}

func TestGetMotd(t *testing.T) {
	t.Parallel()

	c, base := newClient(t)
	got, err := c.GetMotd(t.Context(), nil)

	require.NoError(t, err)
	assert.Equal(t, new("hello"), got)
	assert.Equal(t, "text/plain", mediaType(t, base+"/motd"))
}

func TestGetProfile(t *testing.T) {
	t.Parallel()

	c, base := newClient(t)
	got, err := c.GetProfile(t.Context(), nil)

	require.NoError(t, err)
	assert.Equal(t, "Ada", got.Name)
	require.NotNil(t, got.Avatar)
	data, err := got.Avatar.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "PNG", string(data))
	assert.Equal(t, "a.png", got.Avatar.Name())
	assert.Equal(t, "image/png", got.Avatar.ContentType())
	assert.Equal(t, "multipart/form-data", mediaType(t, base+"/profile"))
}
