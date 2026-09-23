// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package fasthttptest serves a fasthttp handler as an http.Handler, so the shared server tests
// can drive it.
package fasthttptest

import (
	"io"
	"net"
	"net/http"

	"github.com/valyala/fasthttp"
)

// Handler serves h for each request: the request is copied into a fasthttp request context and
// the response out of it.
func Handler(h fasthttp.RequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req fasthttp.Request
		req.Header.SetMethod(r.Method)
		req.SetRequestURI(r.URL.RequestURI())
		req.SetHost(r.Host)
		for key, values := range r.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.SetBody(body)

		var ctx fasthttp.RequestCtx
		ctx.Init(&req, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}, nil)
		h(&ctx)

		ctx.Response.Header.VisitAll(func(key, value []byte) {
			w.Header().Add(string(key), string(value))
		})
		w.WriteHeader(ctx.Response.StatusCode())
		_, _ = w.Write(ctx.Response.Body())
	})
}
