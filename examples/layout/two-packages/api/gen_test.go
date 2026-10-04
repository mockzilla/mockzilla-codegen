// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/examples/layout/two-packages/api"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/two-packages/models"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// shop knows order 1 alone.
type shop struct{}

func (shop) CreateOrder(_ context.Context, opts *api.CreateOrderServiceRequestOptions) (*api.CreateOrderResponseData, error) {
	return api.NewCreateOrderResponseData(&models.Order{ID: "2", Status: models.StatusOpen, Items: opts.Body.Items}), nil
}

func (shop) GetOrder(_ context.Context, opts *api.GetOrderServiceRequestOptions) (*api.GetOrderResponseData, error) {
	if opts.PathParams.ID != "1" {
		return api.NewGetOrderResponseData404(&models.Problem{Detail: "no such order"}), nil
	}
	return api.NewGetOrderResponseData200(&models.Order{ID: "1", Status: models.StatusPaid}), nil
}

func TestAcrossPackages(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(api.NewRouter(shop{}))
	t.Cleanup(srv.Close)
	c, err := api.NewClient(srv.URL)
	require.NoError(t, err)
	ctx := t.Context()

	order, err := c.GetOrder(ctx, &api.GetOrderRequestOptions{PathParams: &models.GetOrderPathParams{ID: "1"}})
	require.NoError(t, err)
	assert.Equal(t, &models.Order{ID: "1", Status: models.StatusPaid}, order)

	_, err = c.GetOrder(ctx, &api.GetOrderRequestOptions{PathParams: &models.GetOrderPathParams{ID: "9"}})
	var apiErr *runtime.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)

	items := []models.Item{{Sku: "tea", Quantity: 2}}
	created, err := c.CreateOrder(ctx, &api.CreateOrderRequestOptions{Body: &models.CreateOrderRequestBody{Items: items}})
	require.NoError(t, err)
	assert.Equal(t, &models.Order{ID: "2", Status: models.StatusOpen, Items: items}, created)
}
