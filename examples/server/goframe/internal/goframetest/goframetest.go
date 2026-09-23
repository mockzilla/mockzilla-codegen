// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package goframetest serves a GoFrame server as an http.Handler, so the shared server tests
// can drive it. GoFrame serves requests once the server is started, so each server is started
// on a free port for the test and stopped with it.
package goframetest

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/stretchr/testify/require"
)

// Handler starts s on a free port, stops it when t ends, and forwards each request to it.
func Handler(t *testing.T, s *ghttp.Server) http.Handler {
	t.Helper()

	s.SetDumpRouterMap(false)
	s.SetPort(0)
	require.NoError(t, s.Start())
	t.Cleanup(func() { require.NoError(t, s.Shutdown()) })
	base := "http://127.0.0.1:" + strconv.Itoa(s.GetListenedPort())

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, base+r.URL.RequestURI(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req.Header = r.Header.Clone()
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer res.Body.Close()
		for key, values := range res.Header {
			w.Header()[key] = values
		}
		w.WriteHeader(res.StatusCode)
		_, _ = io.Copy(w, res.Body)
	})
}
