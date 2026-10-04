// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notFound stands in for an error type of the spec.
type notFound struct {
	Message string `json:"message"`
}

func (e notFound) Error() string {
	return e.Message
}

// badError cannot be written as JSON.
type badError struct {
	C chan int `json:"c"`
}

func (badError) Error() string {
	return "bad"
}

type validated struct{ err error }

func (v validated) Validate() error {
	return v.err
}

type responseValidated struct{ err error }

func (v responseValidated) Validate() error {
	return errors.New("wrong method")
}

func (v responseValidated) ValidateResponse() error {
	return v.err
}

func TestHandlerError(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	tests := []struct {
		name       string
		err        HandlerError
		wantText   string
		wantStatus int
	}{
		{name: "Parse", err: HandlerError{Kind: ErrorParse, ParamName: "id", ParamLocation: "path", Err: cause}, wantText: `invalid path parameter "id": boom`, wantStatus: 400},
		{name: "Decode", err: HandlerError{Kind: ErrorDecode, Err: cause}, wantText: "invalid request body: boom", wantStatus: 400},
		{name: "Decode with a status", err: HandlerError{Kind: ErrorDecode, Status: 415, Err: cause}, wantText: "invalid request body: boom", wantStatus: 415},
		{name: "Validation", err: HandlerError{Kind: ErrorValidation, Err: cause}, wantText: "invalid request: boom", wantStatus: 400},
		{name: "Service hides the cause", err: HandlerError{Kind: ErrorService, Err: cause}, wantText: "internal server error", wantStatus: 500},
		{name: "Response", err: HandlerError{Kind: ErrorResponse, Err: cause}, wantText: "invalid response: boom", wantStatus: 500},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantText, tc.err.Error())
			assert.Equal(t, tc.wantStatus, tc.err.StatusCode())
			assert.Same(t, cause, tc.err.Unwrap())
			require.ErrorIs(t, &tc.err, cause)
		})
	}
	assert.Equal(t, "validation", ErrorValidation.String())
	assert.Equal(t, "response", ErrorResponse.String())
	assert.Equal(t, "unknown", ErrorKind(9).String())
}

func TestDefaultErrorHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		accept          string
		contentType     string
		err             error
		wantContentType string
		wantBody        string
	}{
		{name: "Under the JSON media type set", contentType: "application/problem+json", err: notFound{Message: "gone"}, wantContentType: "application/problem+json", wantBody: `{"message":"gone"}`},
		{name: "A media type set that is no JSON", contentType: "application/xml", err: notFound{Message: "gone"}, wantContentType: "application/json", wantBody: `{"message":"gone"}`},
		{name: "Handler error as JSON", err: &HandlerError{Kind: ErrorDecode, Err: errors.New("bad")}, wantContentType: "application/json", wantBody: `{"error":"invalid request body: bad"}`},
		{name: "Typed error as its own JSON", accept: "application/json", err: notFound{Message: "gone"}, wantContentType: "application/json", wantBody: `{"message":"gone"}`},
		{name: "Wildcard accept", accept: "text/html, */*;q=0.1", err: notFound{Message: "gone"}, wantContentType: "application/json", wantBody: `{"message":"gone"}`},
		{name: "Text", accept: "text/plain", err: notFound{Message: "gone"}, wantContentType: "text/plain; charset=utf-8", wantBody: "gone"},
		{name: "Plain error", err: errors.New("plain"), wantContentType: "application/json", wantBody: `{"error":"plain"}`},
		{name: "Error that has no JSON", err: badError{}, wantContentType: "application/json", wantBody: `{"error":"bad"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			if tc.contentType != "" {
				w.Header().Set("Content-Type", tc.contentType)
			}
			DefaultErrorHandler{}.HandleError(w, r, http.StatusTeapot, tc.err)

			assert.Equal(t, http.StatusTeapot, w.Code)
			assert.Equal(t, tc.wantContentType, w.Header().Get("Content-Type"))
			assert.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

func TestDefaultErrorHandlerAfterACut(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	err := &HandlerError{Kind: ErrorService, Err: fmt.Errorf("%w: gone", ErrResponseCut)}
	DefaultErrorHandler{}.HandleError(w, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusInternalServerError, err)

	assert.Empty(t, w.Header())
	assert.Zero(t, w.Body.Len())
}

func TestErrorHandlerFunc(t *testing.T) {
	t.Parallel()

	var got int
	f := ErrorHandlerFunc(func(_ http.ResponseWriter, _ *http.Request, status int, _ error) { got = status })
	f.HandleError(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), 503, nil)

	assert.Equal(t, 503, got)
}

func TestAsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		want   notFound
		wantOK bool
	}{
		{name: "A value of the type", err: notFound{Message: "gone"}, want: notFound{Message: "gone"}, wantOK: true},
		{name: "A pointer to the type", err: &notFound{Message: "gone"}, want: notFound{Message: "gone"}, wantOK: true},
		{name: "Wrapped", err: fmt.Errorf("lookup: %w", &notFound{Message: "gone"}), want: notFound{Message: "gone"}, wantOK: true},
		{name: "A nil pointer to the type", err: (*notFound)(nil)},
		{name: "Another type", err: badError{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := AsError[notFound](tc.err)

			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestContentTypeError(t *testing.T) {
	t.Parallel()

	err := ContentTypeError("text/csv")

	require.ErrorIs(t, err, ErrContentType)
	assert.EqualError(t, err, "unsupported content type: text/csv")
}

func TestValidateResponse(t *testing.T) {
	t.Parallel()

	fail := errors.New("fail")

	require.ErrorIs(t, ValidateResponse(validated{err: fail}), fail)
	require.ErrorIs(t, ValidateResponse(responseValidated{err: fail}), fail)
	require.NoError(t, ValidateResponse("text"))
}
