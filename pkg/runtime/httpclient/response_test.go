// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpclient

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type pageHeaders struct {
	Total *int     `json:"X-Total"`
	Tags  []string `json:"X-Tags"`
	skip  string   //nolint:unused // left alone by the decoder
}

// envelope is what a generated response type looks like.
type envelope struct {
	JSON200     *rgb
	Text200     *string
	PDF200      *runtime.File
	Bytes2XX    []byte
	Any200      any
	JSON404     *notFound
	JSONDefault *notFound
	Headers200  *pageHeaders
}

// notFound stands in for an error type of the spec.
type notFound struct {
	Message string `json:"message"`
}

func (e notFound) Error() string {
	return e.Message
}

func response(status int, contentType string, header http.Header) *http.Response {
	h := http.Header{}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	for key, values := range header {
		h[key] = values
	}
	return &http.Response{StatusCode: status, Header: h}
}

func envelopeTargets(e *envelope) []ResponseTarget {
	return []ResponseTarget{
		{Status: "200", MediaType: "text/plain", Dst: &e.Text200},
		{Status: "200", MediaType: "application/json", Dst: &e.JSON200},
		{Status: "200", MediaType: "application/pdf", Dst: &e.PDF200},
		{Status: "200", MediaType: "*/*", Dst: &e.Any200},
		{Status: "200", IsHeaders: true, Dst: &e.Headers200},
		{Status: "2XX", MediaType: "image/*", Dst: &e.Bytes2XX},
		{Status: "404", MediaType: "application/notFound+json", Dst: &e.JSON404},
		{Status: "default", MediaType: "application/json", Dst: &e.JSONDefault},
	}
}

func TestDecodeResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		res     *http.Response
		body    string
		want    envelope
		wantErr string
	}{
		{
			name: "A JSON body and the typed headers of its status",
			res:  response(200, "application/json; charset=utf-8", http.Header{"X-Total": {"5"}, "X-Tags": {"a,b"}}),
			body: `{"R":1,"G":2,"B":3}`,
			want: envelope{JSON200: &rgb{R: 1, G: 2, B: 3}, Headers200: &pageHeaders{Total: runtime.Ptr(5), Tags: []string{"a", "b"}}},
		},
		{name: "A text body", res: response(200, "text/plain", nil), body: "pong", want: envelope{Text200: runtime.Ptr("pong"), Headers200: &pageHeaders{}}},
		{name: "A binary body as it came", res: response(200, "application/pdf", nil), body: "%PDF-1.7", want: envelope{PDF200: runtime.Ptr(runtime.NewFile([]byte("%PDF-1.7"), "", "application/pdf")), Headers200: &pageHeaders{}}},
		{name: "A JSON null leaves the pointer nil", res: response(200, "application/json", nil), body: "null", want: envelope{Headers200: &pageHeaders{}}},
		{name: "A media type only the wildcard takes", res: response(200, "text/html", nil), body: `"x"`, want: envelope{Any200: "x", Headers200: &pageHeaders{}}},
		{name: "No media type takes the JSON target, wherever it is listed", res: response(200, "", nil), body: `{"R":1}`, want: envelope{JSON200: &rgb{R: 1}, Headers200: &pageHeaders{}}},
		{name: "A range takes bytes under its wildcard", res: response(201, "image/png", nil), body: "png", want: envelope{Bytes2XX: []byte("png")}},
		{name: "A range does not take another media type", res: response(201, "text/plain", nil), body: "x"},
		{name: "An error status with its JSON family", res: response(404, "application/json", nil), body: `{"message":"gone"}`, want: envelope{JSON404: &notFound{Message: "gone"}}},
		{name: "Default takes what nothing else does", res: response(500, "application/json", nil), body: `{"message":"boom"}`, want: envelope{JSONDefault: &notFound{Message: "boom"}}},
		{name: "An empty body sets nothing but the headers", res: response(200, "application/json", nil), want: envelope{Headers200: &pageHeaders{}}},
		{name: "A status nothing documents", res: response(302, "text/plain", nil), body: "x"},
		{name: "A body that is no JSON", res: response(404, "application/json", nil), body: "<html>", wantErr: "invalid character '<' looking for beginning of value"},
		{name: "A header that does not decode", res: response(200, "", http.Header{"X-Total": {"x"}}), wantErr: `invalid response header X-Total: invalid parameter value: "x" is no int`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got envelope
			err := DecodeResponse(tc.res, []byte(tc.body), envelopeTargets(&got))

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeFormResponse(t *testing.T) {
	t.Parallel()

	var got *rgb
	targets := []ResponseTarget{{Status: "200", MediaType: "application/x-www-form-urlencoded", Dst: &got}}

	require.NoError(t, DecodeResponse(response(200, "application/x-www-form-urlencoded", nil), []byte("R=1&G=2"), targets))
	assert.Equal(t, &rgb{R: 1, G: 2}, got)
	require.Error(t, DecodeResponse(response(200, "application/x-www-form-urlencoded", nil), []byte("R=%zz"), targets))
}

func TestDecodeSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		res     *http.Response
		body    string
		want    envelope
		wantErr error
		wantMsg string
		wantAPI *APIError
	}{
		{name: "The body of a 2xx", res: response(200, "application/json", nil), body: `{"R":1}`, want: envelope{JSON200: &rgb{R: 1}}},
		{name: "A 2xx without a body", res: response(204, "", nil)},
		{name: "A listed 2xx whose body is not read", res: response(204, "application/json", nil), body: `{"R":1}`},
		{
			name:    "A 2xx nothing lists",
			res:     response(202, "application/json", nil),
			body:    `{}`,
			wantMsg: "unexpected status 202 Accepted",
			wantAPI: &APIError{StatusCode: 202, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)},
		},
		{name: "A 2xx in a media type no target takes", res: response(200, "text/html", nil), body: "<html>", wantErr: runtime.ErrContentType, wantMsg: "unsupported content type: text/html"},
		{
			name:    "An error status decoded into its error type",
			res:     response(404, "application/notFound+json", nil),
			body:    `{"message":"gone"}`,
			wantMsg: "unexpected status 404 Not Found: gone",
			wantAPI: &APIError{StatusCode: 404, Header: http.Header{"Content-Type": {"application/notFound+json"}}, Body: []byte(`{"message":"gone"}`), Err: &notFound{Message: "gone"}},
		},
		{
			name:    "An error status whose body does not decode",
			res:     response(404, "application/json", nil),
			body:    "<html>",
			wantMsg: "unexpected status 404 Not Found",
			wantAPI: &APIError{StatusCode: 404, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte("<html>")},
		},
		{
			name:    "An error status without a target",
			res:     response(600, "", nil),
			wantMsg: "unexpected status 600",
			wantAPI: &APIError{StatusCode: 600, Header: http.Header{}},
		},
		{
			name:    "An error status whose target is no error type",
			res:     response(499, "text/plain", nil),
			body:    "x",
			wantMsg: "unexpected status 499",
			wantAPI: &APIError{StatusCode: 499, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte("x")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got envelope
			list := []ResponseTarget{
				{Status: "200", MediaType: "application/json", Dst: &got.JSON200},
				{Status: "204"},
				{Status: "404", MediaType: "application/json", Dst: new(notFound)},
				{Status: "499", MediaType: "text/plain", Dst: new(string)},
			}

			var body []byte
			if tc.body != "" {
				body = []byte(tc.body)
			}

			err := DecodeSuccess(tc.res, body, list)

			switch {
			case tc.wantAPI != nil:
				var apiErr *APIError
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, tc.wantAPI, apiErr)
				require.EqualError(t, err, tc.wantMsg)
				if tc.wantAPI.Err != nil {
					var typed *notFound
					require.ErrorAs(t, err, &typed)
					assert.Equal(t, tc.wantAPI.Err, typed)
				}
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
				require.EqualError(t, err, tc.wantMsg)
			default:
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestDecodeSuccessOfA2xx(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    int
		targets func(*envelope) []ResponseTarget
		want    envelope
		wantAPI bool
	}{
		{
			name: "Default lists no 2xx",
			code: 202,
			targets: func(e *envelope) []ResponseTarget {
				return []ResponseTarget{{Status: "200", MediaType: "application/json", Dst: &e.JSON200}, {Status: "default", MediaType: "application/json", Dst: &e.JSONDefault}}
			},
			wantAPI: true,
		},
		{
			name: "A range lists every 2xx",
			code: 203,
			targets: func(e *envelope) []ResponseTarget {
				return []ResponseTarget{{Status: "2xx", MediaType: "application/json", Dst: &e.JSON200}}
			},
			want: envelope{JSON200: &rgb{R: 1}},
		},
		{
			name: "A code goes before the range",
			code: 201,
			targets: func(e *envelope) []ResponseTarget {
				return []ResponseTarget{{Status: "2XX", MediaType: "application/json", Dst: &e.JSON200}, {Status: "201"}}
			},
		},
		{
			name: "Without a 2xx target any 2xx is taken",
			code: 202,
			targets: func(e *envelope) []ResponseTarget {
				return []ResponseTarget{{Status: "404", MediaType: "application/json", Dst: &e.JSON404}}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got envelope
			err := DecodeSuccess(response(tc.code, "application/json", nil), []byte(`{"R":1}`), tc.targets(&got))

			assert.Equal(t, tc.want, got)
			if !tc.wantAPI {
				require.NoError(t, err)
				return
			}
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, &APIError{StatusCode: tc.code, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"R":1}`)}, apiErr)
		})
	}
}

func TestDecodeUnderWildcard(t *testing.T) {
	t.Parallel()

	res := response(200, "application/json", nil)
	tests := []struct {
		name      string
		mediaType string
		body      string
		dst       any
		want      any
	}{
		{name: "Text as it came", mediaType: "*/*", body: `"A+"`, dst: new(string), want: new(`"A+"`)},
		{name: "Text that is no JSON string", mediaType: "*/*", body: `{"type":"A+"}`, dst: new(*string), want: new(new(`{"type":"A+"}`))},
		{name: "Bytes under a range", mediaType: "application/*", body: `{"a":1}`, dst: new([]byte), want: new([]byte(`{"a":1}`))},
		{name: "A file", mediaType: "*/*", body: `{"a":1}`, dst: new(*runtime.File), want: new(new(runtime.NewFile([]byte(`{"a":1}`), "", "application/json")))},
		{name: "A struct as JSON", mediaType: "*/*", body: `{"R":1}`, dst: new(rgb), want: &rgb{R: 1}},
		{name: "Text under JSON is a JSON string", mediaType: "application/json", body: `"A+"`, dst: new(string), want: new("A+")},
		{name: "Bytes under JSON are base64", mediaType: "application/json", body: `"aGk="`, dst: new([]byte), want: new([]byte("hi"))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := DecodeSuccess(res, []byte(tc.body), []ResponseTarget{{Status: "200", MediaType: tc.mediaType, Dst: tc.dst}})

			require.NoError(t, err)
			assert.Equal(t, tc.want, tc.dst)
		})
	}
}

func TestDecodeEdges(t *testing.T) {
	t.Parallel()

	res := response(200, "application/json", nil)
	tests := []struct {
		name    string
		target  ResponseTarget
		wantErr string
	}{
		{name: "A body target that is no pointer", target: ResponseTarget{Status: "200", MediaType: "application/json", Dst: 1}, wantErr: "invalid parameter value: the target must be a pointer"},
		{name: "A headers target that is no pointer", target: ResponseTarget{Status: "200", IsHeaders: true, Dst: 1}, wantErr: "invalid parameter value: the target must be a pointer"},
		{name: "A headers target that is no struct", target: ResponseTarget{Status: "200", IsHeaders: true, Dst: new(int)}, wantErr: "invalid parameter value: typed headers need a struct, not int"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.EqualError(t, DecodeResponse(res, []byte("{}"), []ResponseTarget{tc.target}), tc.wantErr)
		})
	}
}

func TestStatusRank(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
		code   int
		want   int
	}{
		{name: "The code itself", status: "404", code: 404, want: 3},
		{name: "Its range", status: "4xx", code: 404, want: 2},
		{name: "Default", status: "Default", code: 404, want: 1},
		{name: "Another code", status: "200", code: 404},
		{name: "Another range", status: "2XX", code: 404},
		{name: "Not a status", status: "XXX", code: 404},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, statusRank(tc.status, tc.code))
		})
	}
}

func TestAPIErrorUnwrapsToNothingWithoutATypedError(t *testing.T) {
	t.Parallel()

	err := error(&APIError{StatusCode: 500})

	var typed *notFound
	assert.NotErrorAs(t, err, &typed)
	assert.EqualError(t, err, "unexpected status 500 Internal Server Error")
}
