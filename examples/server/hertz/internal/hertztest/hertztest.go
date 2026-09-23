// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package hertztest serves a hertz engine as an http.Handler, so the shared server tests can
// drive it.
package hertztest

import (
	"bytes"
	"io"
	"net/http"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"
)

// Handler serves engine for each request through hertz's test utilities.
func Handler(engine *route.Engine) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var headers []ut.Header
		for key, values := range r.Header {
			for _, value := range values {
				headers = append(headers, ut.Header{Key: key, Value: value})
			}
		}

		res := ut.PerformRequest(engine, r.Method, r.URL.RequestURI(), &ut.Body{Body: bytes.NewReader(body), Len: len(body)}, headers...).Result()

		res.Header.VisitAll(func(key, value []byte) {
			w.Header().Add(string(key), string(value))
		})
		w.WriteHeader(res.StatusCode())
		_, _ = w.Write(res.Body())
	})
}
