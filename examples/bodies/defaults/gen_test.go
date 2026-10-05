// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package defaults

import (
	"cmp"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers with the order as it got it.
type service struct{}

func (service) AddOrder(_ context.Context, opts *AddOrderServiceRequestOptions) (*AddOrderResponseData, error) {
	return NewAddOrderResponseData(cmp.Or(opts.BodyJSON, opts.BodyForm)), nil
}

func TestAddOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{name: "A missing property gets its default", contentType: "application/json", body: `{"item":"pen"}`, want: `{"item":"pen","qty":1}`},
		{name: "A nested object gets its defaults when it is sent", contentType: "application/json", body: `{"item":"pen","gift":{}}`, want: `{"item":"pen","qty":1,"gift":{"wrap":true}}`},
		{name: "Without validation nothing is checked", contentType: "application/json", body: `{"item":null,"x":1}`, want: `{"item":"","qty":1}`},
		{name: "A form gets its defaults", contentType: "application/x-www-form-urlencoded", body: "item=pen&gift[card]=hi", want: `{"item":"pen","qty":1,"gift":{"wrap":true,"card":"hi"}}`},
	}

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := http.Post(srv.URL+"/orders", tc.contentType, strings.NewReader(tc.body))
			require.NoError(t, err)
			defer res.Body.Close()
			data, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.JSONEq(t, tc.want, string(data))
		})
	}
}
