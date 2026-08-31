// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrorKind says which step of handling a request failed.
type ErrorKind int

const (
	// ErrorParse is a parameter that could not be read.
	ErrorParse ErrorKind = iota
	// ErrorDecode is a body that could not be read.
	ErrorDecode
	// ErrorValidation is a request the spec rejects.
	ErrorValidation
	// ErrorService is an error the service returned.
	ErrorService
	// ErrorResponse is a response the spec rejects.
	ErrorResponse
)

var kindTexts = []string{"parse", "decode", "validation", "service", "response"}

// HandlerError is what a generated handler passes to the error handler when a request cannot be
// served. Status is the code to answer with, 0 for the one the kind implies. Err is the cause.
type HandlerError struct {
	Kind          ErrorKind
	OperationID   string
	ParamName     string
	ParamLocation string
	Status        int
	Err           error
}

// ErrorHandler writes the response of a failed request. err is a *HandlerError, or an error type
// of the spec returned by the service.
type ErrorHandler interface {
	HandleError(w http.ResponseWriter, r *http.Request, status int, err error)
}

// ErrorHandlerFunc is an ErrorHandler made of one function.
type ErrorHandlerFunc func(w http.ResponseWriter, r *http.Request, status int, err error)

// DefaultErrorHandler writes an error as {"error": "..."}, or an error type of the spec as its own
// JSON, when the request accepts JSON, and as text otherwise.
type DefaultErrorHandler struct{}

func (k ErrorKind) String() string {
	if int(k) < len(kindTexts) {
		return kindTexts[k]
	}
	return "unknown"
}

// Error describes the failure without the service's own error text, which may hold internals.
func (e *HandlerError) Error() string {
	switch e.Kind {
	case ErrorParse:
		return fmt.Sprintf("invalid %s parameter %q: %v", e.ParamLocation, e.ParamName, e.Err)
	case ErrorDecode:
		return fmt.Sprintf("invalid request body: %v", e.Err)
	case ErrorValidation:
		return fmt.Sprintf("invalid request: %v", e.Err)
	case ErrorResponse:
		return fmt.Sprintf("invalid response: %v", e.Err)
	case ErrorService:
	}
	return "internal server error"
}

func (e *HandlerError) Unwrap() error {
	return e.Err
}

// StatusCode is Status, or the code the kind implies: 400 for a bad request, 500 for the service
// and its response.
func (e *HandlerError) StatusCode() int {
	switch {
	case e.Status != 0:
		return e.Status
	case e.Kind == ErrorService, e.Kind == ErrorResponse:
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

func (f ErrorHandlerFunc) HandleError(w http.ResponseWriter, r *http.Request, status int, err error) {
	f(w, r, status, err)
}

func (DefaultErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, status int, err error) {
	if !AcceptsJSON(r.Header.Get("Accept")) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_ = WriteBytes(w, status, []byte(err.Error()))
		return
	}

	var herr *HandlerError
	data, jsonErr := json.Marshal(err)
	if errors.As(err, &herr) || jsonErr != nil || string(data) == "{}" {
		data, _ = json.Marshal(map[string]string{"error": err.Error()}) // strings always marshal
	}
	w.Header().Set("Content-Type", "application/json")
	_ = WriteBytes(w, status, data)
}

// AcceptsJSON reports an Accept header that takes JSON: empty, */*, application/*, or a JSON type.
func AcceptsJSON(accept string) bool {
	if accept == "" {
		return true
	}
	for part := range strings.SplitSeq(accept, ",") {
		mediaType, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if mediaType == "*/*" || mediaType == "application/*" || IsJSON(mediaType) {
			return true
		}
	}
	return false
}

// AsError returns the T in err's chain, as a value or behind a pointer, when there is one.
func AsError[T error](err error) (T, bool) {
	var v T
	if errors.As(err, &v) {
		return v, true
	}
	// vet cannot see that *T implements error; errors.As checks it at run time.
	var p *T
	if errors.As(err, any(&p)) && p != nil {
		return *p, true
	}
	return v, false
}

// ContentTypeError is the error of a request body in a media type the operation does not take.
func ContentTypeError(mediaType string) error {
	return fmt.Errorf("%w: %s", ErrContentType, mediaType)
}

// ValidateResponse checks v with its ValidateResponse method, else its Validate method, when it
// has one.
func ValidateResponse(v any) error {
	switch v := v.(type) {
	case interface{ ValidateResponse() error }:
		return v.ValidateResponse()
	case interface{ Validate() error }:
		return v.Validate()
	}
	return nil
}
