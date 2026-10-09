// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Middleware that runs inside a handler of a generated server, once its operation is known.

package httpserver

import (
	"context"
	"net/http"
	"slices"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type serveKey struct{}

// Chain is middleware built once, which each request runs its own handler inside.
type Chain struct {
	handler http.Handler
}

// NewChain wraps mw around the handler each request carries, outermost first.
func NewChain(mw ...func(http.Handler) http.Handler) Chain {
	if len(mw) == 0 {
		return Chain{}
	}

	var h http.Handler = http.HandlerFunc(serveCarried)
	for _, m := range slices.Backward(mw) {
		h = m(h)
	}
	return Chain{handler: h}
}

// Serve puts id on the request's context as the operation name, then runs serve inside the
// middleware.
func (c Chain) Serve(w http.ResponseWriter, r *http.Request, id string, serve http.HandlerFunc) {
	ctx := runtime.WithOperationID(r.Context(), id)
	if c.handler == nil {
		serve(w, r.WithContext(ctx))
		return
	}
	// The middleware is built once, so the handler of this request travels on its context.
	c.handler.ServeHTTP(w, r.WithContext(context.WithValue(ctx, serveKey{}, serve)))
}

func serveCarried(w http.ResponseWriter, r *http.Request) {
	serve, _ := r.Context().Value(serveKey{}).(http.HandlerFunc)
	if serve == nil {
		panic(ErrContextLost)
	}
	serve(w, r)
}
