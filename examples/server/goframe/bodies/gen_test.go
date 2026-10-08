// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/examples/server/goframe/internal/goframetest"
	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
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

func (mirror) PutFile(_ context.Context, opts *PutFileServiceRequestOptions) (*PutFileResponseData, error) {
	return NewPutFileResponseData(opts.Body), nil
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

func (mirror) GetForm(context.Context, *GetFormServiceRequestOptions) (*GetFormResponseData, error) {
	return NewGetFormResponseData(&Note{Text: "hi", Stars: new(2)}), nil
}

func (mirror) GetQuote(context.Context, *GetQuoteServiceRequestOptions) (*GetQuoteResponseData, error) {
	return NewGetQuoteResponseData(new("hi")), nil
}

func (mirror) GetCount(context.Context, *GetCountServiceRequestOptions) (*GetCountResponseData, error) {
	return NewGetCountResponseData(new(3)), nil
}

func (mirror) ListNotes(context.Context, *ListNotesServiceRequestOptions) (*ListNotesResponseData, error) {
	return NewListNotesResponseData(slices.Values([]Note{{Text: "a"}, {Text: "b", Stars: new(1)}})), nil
}

func TestBodies(t *testing.T) {
	t.Parallel()

	servertest.Run(t, goframetest.Handler(t, NewRouter(mirror{})), servertest.Bodies)
}

func TestMultipart(t *testing.T) {
	t.Parallel()

	servertest.Multipart(t, goframetest.Handler(t, NewRouter(mirror{})))
}

func TestJSONOptions(t *testing.T) {
	t.Parallel()

	upper := func(data []byte, v any) error {
		return json.Unmarshal(bytes.ToUpper(data), v)
	}
	indent := func(v any) ([]byte, error) {
		return json.MarshalIndent(v, "", " ")
	}
	tests := []struct {
		name     string
		opt      ServerOption
		wantBody string
	}{
		{name: "A decoder reads the request", opt: WithJSONDecoder(upper), wantBody: `{"text":"HI"}`},
		{name: "An encoder writes the response", opt: WithJSONEncoder(indent), wantBody: "{\n \"text\": \"hi\"\n}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest("POST", "/json", strings.NewReader(`{"text":"hi"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			goframetest.Handler(t, NewRouter(mirror{}, tc.opt)).ServeHTTP(rec, req)

			assert.Equal(t, 200, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}
