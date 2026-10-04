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

	"github.com/mockzilla/mockzilla-codegen/examples/layout/service-apart/api"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/service-apart/models"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/service-apart/orders"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/service-apart/service"
)

// shop answers GetOrder on top of the scaffolded service, which imports the service package and
// not the router.
type shop struct {
	*orders.Service
}

func (shop) GetOrder(_ context.Context, opts *service.GetOrderServiceRequestOptions) (*service.GetOrderResponseData, error) {
	if opts.PathParams.ID != "1" {
		return service.NewGetOrderResponseData404(&models.Problem{Detail: "no such order"}), nil
	}
	return service.NewGetOrderResponseData200(&models.Order{ID: "1", Status: models.StatusPaid}), nil
}

func TestServiceApart(t *testing.T) {
	t.Parallel()

	router := api.NewRouter(shop{Service: orders.NewService()})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/1", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id":"1","status":"paid"}`, rec.Body.String())

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orders/9", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"detail":"no such order"}`, rec.Body.String())
}
