// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package orders

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// shop knows order 1 alone.
type shop struct{}

func (shop) CreateOrder(_ context.Context, opts *CreateOrderServiceRequestOptions) (*CreateOrderResponseData, error) {
	return NewCreateOrderResponseData(&Order{ID: "2", Status: StatusOpen, Items: opts.Body.Items}), nil
}

func (shop) GetOrder(_ context.Context, opts *GetOrderServiceRequestOptions) (*GetOrderResponseData, error) {
	if opts.PathParams.ID != "1" {
		return NewGetOrderResponseData404(&Problem{Detail: "no such order"}), nil
	}
	return NewGetOrderResponseData200(&Order{ID: "1", Status: StatusPaid}), nil
}

func TestAcrossFiles(t *testing.T) {
	t.Parallel()

	router := NewRouter(shop{})
	serve := func(method, target, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := serve(http.MethodGet, "/orders/1?expand=items", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"id":"1","status":"paid"}`, rec.Body.String())

	rec = serve(http.MethodGet, "/orders/9", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.JSONEq(t, `{"detail":"no such order"}`, rec.Body.String())

	rec = serve(http.MethodPost, "/orders", `{"items":[{"sku":"tea","quantity":2}]}`)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"id":"2","status":"open","items":[{"sku":"tea","quantity":2}]}`, rec.Body.String())
}
