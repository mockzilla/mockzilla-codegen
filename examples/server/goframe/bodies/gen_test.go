// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/examples/server/goframe/internal/goframetest"
	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// mirror answers with what it received.
type mirror struct{}

func (mirror) PostJSON(_ context.Context, opts *PostJSONServiceRequestOptions) (*PostJSONResponseData, error) {
	return NewPostJSONResponseData(opts.Body), nil
}

func (mirror) PostForm(_ context.Context, opts *PostFormServiceRequestOptions) (*PostFormResponseData, error) {
	return NewPostFormResponseData(opts.Body), nil
}

func (mirror) Upload(_ context.Context, opts *UploadServiceRequestOptions) (*UploadResponseData, error) {
	content, err := opts.Body.File.Bytes()
	if err != nil {
		return nil, err
	}
	return NewUploadResponseData(UploadResponse200{
		"title": opts.Body.Title,
		"file":  string(content),
		"name":  opts.Body.File.Name(),
		"tags":  opts.Body.Tags,
		"meta":  opts.Body.Meta,
	}), nil
}

func (mirror) PostText(_ context.Context, opts *PostTextServiceRequestOptions) (*PostTextResponseData, error) {
	if opts.BodyOctetStream != nil {
		return NewPostTextResponseData(new("bytes: " + string(opts.BodyOctetStream))), nil
	}
	if opts.BodyText == nil {
		return NewPostTextResponseData(new("nothing")), nil
	}
	return NewPostTextResponseData(new("text: " + *opts.BodyText)), nil
}

func (mirror) PostAny(_ context.Context, opts *PostAnyServiceRequestOptions) (*PostAnyResponseData, error) {
	switch {
	case opts.BodyXML != nil:
		return NewPostAnyResponseData(new("xml: " + *opts.BodyXML)), nil
	case opts.BodyTextXML != nil:
		return NewPostAnyResponseData(new("text xml: " + *opts.BodyTextXML)), nil
	}
	return NewPostAnyResponseData(new("any: " + string(opts.BodyAny))), nil
}

func TestBodies(t *testing.T) {
	t.Parallel()

	servertest.Run(t, goframetest.Handler(t, NewRouter(mirror{})), servertest.Bodies)
}

func TestMultipart(t *testing.T) {
	t.Parallel()

	servertest.Multipart(t, goframetest.Handler(t, NewRouter(mirror{})))
}

func TestJSONDecoderOption(t *testing.T) {
	t.Parallel()

	strict := func(body io.Reader, dst any, isRequired bool) error {
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		return runtime.DecodeJSON(bytes.NewReader(bytes.ToUpper(data)), dst, isRequired)
	}
	req := httptest.NewRequest("POST", "/json", strings.NewReader(`{"text":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	goframetest.Handler(t, NewRouter(mirror{}, WithJSONDecoder(strict))).ServeHTTP(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, `{"text":"HI"}`, rec.Body.String())
}
