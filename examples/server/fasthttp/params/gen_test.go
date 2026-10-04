// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package params

import (
	"context"
	"testing"

	"github.com/mockzilla/mockzilla-codegen/examples/server/fasthttp/internal/fasthttptest"
	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
)

// mirror answers every operation with the parameters it received.
type mirror struct{}

func (mirror) PathStyles(_ context.Context, opts *PathStylesServiceRequestOptions) (*PathStylesResponseData, error) {
	return NewPathStylesResponseData(Echo{"path": opts.PathParams}), nil
}

func (mirror) QueryStyles(_ context.Context, opts *QueryStylesServiceRequestOptions) (*QueryStylesResponseData, error) {
	return NewQueryStylesResponseData(Echo{"query": opts.Query}), nil
}

func (mirror) HeaderStyles(_ context.Context, opts *HeaderStylesServiceRequestOptions) (*HeaderStylesResponseData, error) {
	return NewHeaderStylesResponseData(Echo{"header": opts.Headers}), nil
}

func (mirror) CookieStyles(_ context.Context, opts *CookieStylesServiceRequestOptions) (*CookieStylesResponseData, error) {
	return NewCookieStylesResponseData(Echo{"cookie": opts.Cookies}), nil
}

func (mirror) Search(_ context.Context, opts *SearchServiceRequestOptions) (*SearchResponseData, error) {
	return NewSearchResponseData(Echo{"search": opts.Filter}), nil
}

func TestStyles(t *testing.T) {
	t.Parallel()

	servertest.Run(t, fasthttptest.Handler((NewRouter(mirror{})).Handler), servertest.Params)
}
