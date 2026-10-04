// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package orders_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	orders "github.com/mockzilla/mockzilla-codegen/examples/layout/client-package/client/v2"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/client-package/models"
)

// TestClientPackage calls an API through the client in folder client/v2, which output.packages
// names orders.
func TestClientPackage(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/orders/1" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"no such order"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"1","status":"paid"}`))
	}))
	t.Cleanup(srv.Close)
	c, err := orders.NewClient(srv.URL)
	require.NoError(t, err)
	ctx := t.Context()

	order, err := c.GetOrder(ctx, &orders.GetOrderRequestOptions{PathParams: &models.GetOrderPathParams{ID: "1"}})
	require.NoError(t, err)
	assert.Equal(t, &models.Order{ID: "1", Status: models.StatusPaid}, order)

	res, err := c.GetOrderWithResponse(ctx, &orders.GetOrderRequestOptions{PathParams: &models.GetOrderPathParams{ID: "9"}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, res.StatusCode())
	assert.Equal(t, &models.Problem{Detail: "no such order"}, res.JSON404)
}
