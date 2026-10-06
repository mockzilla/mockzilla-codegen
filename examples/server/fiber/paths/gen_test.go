// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package paths

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gofiber/fiber/v3/middleware/adaptor"
	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
)

// service answers with the operation and its values, keeps every tag and panics for the name panic.
type service struct {
	mu   sync.Mutex
	tags []string
}

func say(values ...string) *string {
	return new(strings.Join(values, " "))
}

func (s *service) GetName(_ context.Context, opts *GetNameServiceRequestOptions) (*GetNameResponseData, error) {
	if opts.PathParams.Name == "panic" {
		panic("boom")
	}
	if opts.Query.Tag == nil {
		return NewGetNameResponseData(say("getName", opts.PathParams.Name)), nil
	}
	s.mu.Lock()
	s.tags = append(s.tags, *opts.Query.Tag)
	s.mu.Unlock()
	return NewGetNameResponseData(say("getName", opts.PathParams.Name, *opts.Query.Tag)), nil
}

func (*service) DeleteName(_ context.Context, opts *DeleteNameServiceRequestOptions) (*DeleteNameResponseData, error) {
	reason := "without a reason"
	if opts.Body != nil && opts.Body.Reason != nil {
		reason = *opts.Body.Reason
	}
	return NewDeleteNameResponseData(say("deleteName", opts.PathParams.Name, reason)), nil
}

func (*service) TraceName(_ context.Context, opts *TraceNameServiceRequestOptions) (*TraceNameResponseData, error) {
	return NewTraceNameResponseData(say("traceName", opts.PathParams.Name)), nil
}

func (*service) GetNameSlash(_ context.Context, opts *GetNameSlashServiceRequestOptions) (*GetNameSlashResponseData, error) {
	return NewGetNameSlashResponseData(say("getNameSlash", opts.PathParams.Name)), nil
}

func (*service) GetPolicy(_ context.Context, opts *GetPolicyServiceRequestOptions) (*GetPolicyResponseData, error) {
	return NewGetPolicyResponseData(say("getPolicy", opts.PathParams.Name)), nil
}

func (*service) GetChildren(_ context.Context, opts *GetChildrenServiceRequestOptions) (*GetChildrenResponseData, error) {
	return NewGetChildrenResponseData(say("getChildren", opts.PathParams.ID)), nil
}

func (*service) PutBatch(context.Context, *PutBatchServiceRequestOptions) (*PutBatchResponseData, error) {
	return NewPutBatchResponseData(say("putBatch")), nil
}

func (*service) GetGeo(_ context.Context, opts *GetGeoServiceRequestOptions) (*GetGeoResponseData, error) {
	return NewGetGeoResponseData(say("getGeo", opts.PathParams.LatLng)), nil
}

func (*service) GetFile(_ context.Context, opts *GetFileServiceRequestOptions) (*GetFileResponseData, error) {
	return NewGetFileResponseData(say("getFile", opts.PathParams.Name, opts.PathParams.Ext)), nil
}

func (*service) GetProduct(_ context.Context, opts *GetProductServiceRequestOptions) (*GetProductResponseData, error) {
	return NewGetProductResponseData(say("getProduct", opts.PathParams.ID)), nil
}

func (*service) GetMetadata(context.Context, *GetMetadataServiceRequestOptions) (*GetMetadataResponseData, error) {
	return NewGetMetadataResponseData(say("getMetadata")), nil
}

func (*service) GetColon(context.Context, *GetColonServiceRequestOptions) (*GetColonResponseData, error) {
	return NewGetColonResponseData(say("getColon")), nil
}

func TestPaths(t *testing.T) {
	t.Parallel()

	svc := &service{}
	servertest.Run(t, adaptor.FiberApp(NewRouter(svc)), slices.Concat(servertest.Paths, []servertest.Request{
		{Name: "An escaped slash", Path: "/v1/a%2Fb", WantBody: "getName a/b"},
		{Name: "A parameter with a literal after it", Path: "/v1/rex:getIamPolicy", WantBody: "getName rex:getIamPolicy"},
		{Name: "A trailing slash", Path: "/v1/rex/", WantBody: "getName rex"},
		{Name: "TRACE", Method: "TRACE", Path: "/v1/rex", WantBody: "traceName rex"},
		{Name: "A literal segment that begins with a colon", Method: "PUT", Path: "/v1/:batch", WantBody: "putBatch"},
		{Name: "An OPTIONS request that asks for DELETE", Method: "OPTIONS", Path: "/v1/rex", Headers: http.Header{"Access-Control-Request-Method": {"DELETE"}}, WantStatus: 405},
		{Name: "A header that names another path", Path: "/geo/1:2", Headers: http.Header{"X-Url-Path": {"/v1/rex"}}, WantBody: "getGeo 1:2"},
		{Name: "Two parameters in one segment", Path: "/files/a.pdf", WantStatus: 404, WantBody: "404 page not found\n"},
		{Name: "A parameter between parentheses", Path: "/products(5)", WantStatus: 404, WantBody: "404 page not found\n"},
		{Name: "A literal with a dollar sign", Path: "/$metadata", WantBody: "getMetadata"},
		{Name: "A literal with a colon", Path: "/a:b", WantBody: "getColon"},
		{Name: "A panic in the service", Path: "/v1/panic", WantStatus: 500, WantBody: `{"error":"internal server error"}`},
	}))
}

func TestKeepAlive(t *testing.T) {
	t.Parallel()

	svc := &service{}
	servertest.KeepAlive(t, func(ln net.Listener) { _ = NewRouter(svc).Listener(ln) })

	svc.mu.Lock()
	defer svc.mu.Unlock()
	assert.Equal(t, []string{"a", "b", "c"}, svc.tags, "a value the service keeps stays its own after the next request")
}
