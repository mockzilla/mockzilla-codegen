// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Reading the response of a generated client into the target of its status.

package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

var (
	fileType = reflect.TypeFor[runtime.File]()
	anyType  = reflect.TypeFor[any]()
)

// ResponseTarget is a field one documented response is decoded into. Status is the status as the
// spec writes it: 200, 2XX or default; MediaType is the content's and Dst points at the field. With
// IsHeaders, Dst is the struct of the typed headers of the status, read from the response headers.
// Without Dst, the target documents a status whose body is not read.
type ResponseTarget struct {
	Status    string
	MediaType string
	Dst       any
	IsHeaders bool
}

// APIError is a response that a method returning the body of a 2xx response does not take: a
// status outside 2xx, or a 2xx the spec does not list. Err is the body decoded into the error type
// the spec documents for the status, nil without one; it is what errors.As unwraps to.
type APIError struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Err        error
}

// matched is what a response selects among the targets: those of the best status, then the
// body target that takes the response's media type. IsListed reports a target of the status, and
// isUntaken body targets of the status that none takes the media type of.
type matched struct {
	body      *ResponseTarget
	headers   *ResponseTarget
	mediaType string
	isListed  bool
	isUntaken bool
}

func (e *APIError) Error() string {
	msg := "unexpected status " + strconv.Itoa(e.StatusCode)
	if text := http.StatusText(e.StatusCode); text != "" {
		msg += " " + text
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// DecodeResponse fills the targets of a response: those of its status, else of its range, else of
// default, taking the body target whose media type fits the response's best. A status nothing
// documents, a media type no target takes and an empty body leave everything as it is.
func DecodeResponse(res *http.Response, body []byte, targets []ResponseTarget) error {
	m := match(res, targets)
	if m.body != nil && len(body) > 0 {
		if err := decodeBody(body, res.Header, m.body); err != nil {
			return err
		}
	}
	if m.headers != nil {
		return runtime.DecodeHeaders(res.Header, m.headers.Dst)
	}
	return nil
}

// DecodeSuccess is DecodeResponse for a method that returns the body of a 2xx response. A 2xx takes
// the targets of its code or its range only, never default, and one that none lists is an *APIError
// when any target lists a 2xx; without such a target, which is a method that returns no body, it is
// no error. A body in a media type no target of the status takes is an error. A status outside 2xx
// is an *APIError, which carries the body decoded into the target of the status when that is an
// error type.
func DecodeSuccess(res *http.Response, body []byte, targets []ResponseTarget) error {
	if res.StatusCode < 200 || res.StatusCode > 299 {
		apiErr := &APIError{StatusCode: res.StatusCode, Header: res.Header, Body: body}
		m := match(res, targets)
		if m.body != nil && len(body) > 0 {
			if typed, ok := m.body.Dst.(error); ok && decodeBody(body, res.Header, m.body) == nil {
				apiErr.Err = typed
			}
		}
		return apiErr
	}

	successes := slices.DeleteFunc(slices.Clone(targets), func(t ResponseTarget) bool { return !isSuccess(t.Status) })
	m := match(res, successes)
	switch {
	case !m.isListed && len(successes) > 0:
		return &APIError{StatusCode: res.StatusCode, Header: res.Header, Body: body}
	case len(body) == 0:
		return nil
	case m.body != nil:
		return decodeBody(body, res.Header, m.body)
	case m.isUntaken:
		return runtime.ContentTypeError(m.mediaType)
	}
	return nil
}

// match selects the targets of the response's status and the body target of its media type.
func match(res *http.Response, targets []ResponseTarget) matched {
	m := matched{mediaType: runtime.ContentType(res.Header)}
	best := 0
	for _, t := range targets {
		best = max(best, statusRank(t.Status, res.StatusCode))
	}
	if best == 0 {
		return m
	}

	m.isListed = true
	bodyRank, hasBody := 0, false
	for i := range targets {
		t := &targets[i]
		if t.Dst == nil || statusRank(t.Status, res.StatusCode) != best {
			continue
		}
		if t.IsHeaders {
			if m.headers == nil {
				m.headers = t
			}
			continue
		}
		hasBody = true
		if r := mediaRank(t.MediaType, m.mediaType); r > bodyRank {
			bodyRank, m.body = r, t
		}
	}
	m.isUntaken = hasBody && m.body == nil
	return m
}

// statusRank says how well a documented status fits a code: its own code, then its range, then
// default, then not at all.
func statusRank(status string, code int) int {
	s := strings.ToUpper(status)
	switch {
	case s == strconv.Itoa(code):
		return 3
	case s == strconv.Itoa(code/100)+"XX":
		return 2
	case s == "DEFAULT":
		return 1
	}
	return 0
}

// isSuccess reports a documented 2xx status: a code from 200 to 299, or the range 2XX.
func isSuccess(status string) bool {
	code, err := strconv.Atoi(status)
	return err == nil && code >= 200 && code <= 299 || strings.EqualFold(status, "2XX")
}

// mediaRank says how well a documented media type fits the response's: the same one, then JSON
// for JSON, then a wildcard, then not at all. A response without a media type takes any target,
// a JSON one first.
func mediaRank(documented, actual string) int {
	documented = baseMediaType(documented)
	switch {
	case actual == "" && runtime.IsJSON(documented):
		return 2
	case actual == "":
		return 1
	case documented == actual:
		return 4
	case runtime.IsJSON(documented) && runtime.IsJSON(actual):
		return 3
	case documented == "*/*":
		return 2
	case strings.HasSuffix(documented, "/*") && strings.HasPrefix(actual, strings.TrimSuffix(documented, "*")):
		return 2
	}
	return 0
}

// decodeBody reads body into the Dst of t, a pointer, by the type of Dst and the media type in header.
func decodeBody(body []byte, header http.Header, t *ResponseTarget) error {
	target := reflect.ValueOf(t.Dst)
	if target.Kind() != reflect.Pointer || target.IsNil() {
		return fmt.Errorf("%w: the target must be a pointer", runtime.ErrParamValue)
	}

	mediaType := runtime.ContentType(header)
	leaf := target.Type()
	for leaf.Kind() == reflect.Pointer {
		leaf = leaf.Elem()
	}
	switch {
	case runtime.IsJSON(mediaType) && !strings.Contains(t.MediaType, "*"):
	case leaf == fileType:
		allocate(target).Set(reflect.ValueOf(runtime.NewFile(body, "", mediaType)))
		return nil
	case leaf.Kind() == reflect.String:
		allocate(target).SetString(string(body))
		return nil
	case leaf.Kind() == reflect.Slice && leaf.Elem().Kind() == reflect.Uint8:
		allocate(target).SetBytes(body)
		return nil
	case mediaType == "application/x-www-form-urlencoded":
		return runtime.DecodeForm(bytes.NewReader(body), t.Dst, false, nil)
	case mediaType == "multipart/form-data" && leaf != anyType:
		return runtime.DecodeMultipartBody(bytes.NewReader(body), header.Get("Content-Type"), t.Dst, nil)
	case leaf == anyType && mediaType != "" && !runtime.IsJSON(mediaType):
		allocate(target).Set(reflect.ValueOf(untyped(body, mediaType)))
		return nil
	case strings.HasPrefix(mediaType, "text/"):
		return runtime.DecodeTextValue(bytes.NewReader(body), t.Dst, false)
	}
	return json.Unmarshal(body, t.Dst)
}

// untyped is body as a schema without a type holds it outside JSON: text under text/*, else bytes.
func untyped(body []byte, mediaType string) any {
	if strings.HasPrefix(mediaType, "text/") {
		return string(body)
	}
	return body
}

// allocate follows v through pointers, making each nil one point at a new value, and returns what
// it reaches.
func allocate(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
}

func baseMediaType(mediaType string) string {
	mediaType, _, _ = strings.Cut(strings.ToLower(mediaType), ";")
	return strings.TrimSpace(mediaType)
}
