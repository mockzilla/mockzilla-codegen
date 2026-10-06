// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Helpers for routers that serve an http.Handler through a server of another kind.

package runtime

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// DetachRequest copies r with strings of its own, for a server like fasthttp that reuses their memory.
func DetachRequest(r *http.Request) *http.Request {
	out := r.Clone(r.Context())
	out.Method = strings.Clone(r.Method)
	out.Proto = strings.Clone(r.Proto)
	out.Host = strings.Clone(r.Host)
	out.RemoteAddr = strings.Clone(r.RemoteAddr)
	out.RequestURI = strings.Clone(r.RequestURI)
	if u, err := url.ParseRequestURI(out.RequestURI); err == nil {
		out.URL = u
	}
	out.Header = make(http.Header, len(r.Header))
	for key, values := range r.Header {
		own := make([]string, len(values))
		for i, v := range values {
			own[i] = strings.Clone(v)
		}
		out.Header[strings.Clone(key)] = own
	}
	return out
}

// DetachPathValue unescapes v, cut from the raw path on such a server, into memory of its own.
func DetachPathValue(v string) string {
	if out, err := url.PathUnescape(v); err == nil && out != v {
		return out
	}
	return strings.Clone(v)
}

// Recover answers a panic in h with a 500 through eh, the error wrapping ErrPanic.
func Recover(h http.Handler, eh ErrorHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				err := &HandlerError{Kind: ErrorService, Err: fmt.Errorf("%w: %v", ErrPanic, p)}
				eh.HandleError(w, r, err.StatusCode(), err)
			}
		}()
		h.ServeHTTP(w, r)
	})
}
