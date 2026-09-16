// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

const upperHex = "0123456789ABCDEF"

// Doer sends a request, as *http.Client does.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// RequestBuilder puts a request together: the path with its parameters, the query, the headers,
// the cookies and the body. The first error stops the rest and comes back from Build.
type RequestBuilder struct {
	method      string
	path        string
	query       url.Values
	header      http.Header
	cookies     []*http.Cookie
	body        io.Reader
	length      int64
	contentType string
	err         error
}

// NewRequestBuilder starts a request of method to path, a template such as /pets/{id} whose
// placeholders PathParam fills in.
func NewRequestBuilder(method, path string) *RequestBuilder {
	return &RequestBuilder{method: method, path: path, query: url.Values{}, header: http.Header{}}
}

// PathParam fills v into the placeholder of p. A nil value is an error, since paths need every
// parameter.
func (b *RequestBuilder) PathParam(v any, p Param) {
	if b.err != nil {
		return
	}
	if isNil(v) {
		b.err = fmt.Errorf("%w: %s", ErrParamMissing, p.Name)
		return
	}

	value, err := EncodePath(v, p)
	if err != nil {
		b.err = err
		return
	}
	b.path = strings.ReplaceAll(b.path, "{"+p.Name+"}", escapeSegment(value))
}

// QueryParam adds v to the query as p. A nil value is left out, unless p is required.
func (b *RequestBuilder) QueryParam(v any, p Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}
	b.err = EncodeQuery(v, p, b.query)
}

// HeaderParam adds v as the header p. A nil value is left out, unless p is required.
func (b *RequestBuilder) HeaderParam(v any, p Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}

	value, err := EncodeHeader(v, p)
	if err != nil {
		b.err = err
		return
	}
	b.header.Add(p.Name, value)
}

// CookieParam adds v as the cookie p. A nil value is left out, unless p is required.
func (b *RequestBuilder) CookieParam(v any, p Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}

	cookies, err := EncodeCookie(v, p)
	if err != nil {
		b.err = err
		return
	}
	b.cookies = append(b.cookies, cookies...)
}

// JSONBody sends v as JSON under mediaType.
func (b *RequestBuilder) JSONBody(v any, mediaType string) {
	if b.err != nil {
		return
	}
	data, err := json.Marshal(v)
	b.setBody(data, mediaType, err)
}

// FormBody sends v as application/x-www-form-urlencoded, see EncodeForm.
func (b *RequestBuilder) FormBody(v any) {
	if b.err != nil {
		return
	}
	values, err := EncodeForm(v)
	if err != nil {
		b.err = err
		return
	}
	b.setBody([]byte(values.Encode()), "application/x-www-form-urlencoded", nil)
}

// MultipartBody sends v as multipart/form-data, see EncodeMultipart.
func (b *RequestBuilder) MultipartBody(v any) {
	if b.err != nil {
		return
	}
	data, contentType, err := EncodeMultipart(v)
	b.setBody(data, contentType, err)
}

// TextBody sends s as it is under mediaType.
func (b *RequestBuilder) TextBody(s, mediaType string) {
	b.setBody([]byte(s), mediaType, nil)
}

// BytesBody sends data as it is under mediaType.
func (b *RequestBuilder) BytesBody(data []byte, mediaType string) {
	b.setBody(data, mediaType, nil)
}

// FileBody streams f under mediaType, or under the file's own content type when mediaType is
// empty, with its size as the content length when it is known.
func (b *RequestBuilder) FileBody(f File, mediaType string) {
	if b.err != nil {
		return
	}
	rc, err := f.Reader()
	if err != nil {
		b.err = err
		return
	}
	if mediaType == "" {
		mediaType = f.ContentType()
	}
	b.body, b.length, b.contentType = rc, f.Size(), mediaType
}

// Build makes the request against base: the path goes after the base's, the query is encoded and
// sorted, and a File body streams. A placeholder that no PathParam filled is a missing parameter.
func (b *RequestBuilder) Build(ctx context.Context, base *url.URL) (*http.Request, error) {
	if b.err != nil {
		return nil, b.err
	}
	if start := strings.IndexByte(b.path, '{'); start >= 0 {
		name, _, _ := strings.Cut(b.path[start+1:], "}")
		return nil, fmt.Errorf("%w: %s", ErrParamMissing, name)
	}

	u := *base
	u.RawPath = strings.TrimSuffix(base.EscapedPath(), "/") + b.path
	path, err := url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParamValue, err)
	}
	u.Path, u.RawQuery = path, b.query.Encode()

	req, err := http.NewRequestWithContext(ctx, b.method, u.String(), b.body)
	if err != nil {
		return nil, err
	}
	if b.length > 0 {
		req.ContentLength = b.length
	}
	maps.Copy(req.Header, b.header)
	if b.contentType != "" {
		req.Header.Set("Content-Type", b.contentType)
	}
	for _, c := range b.cookies {
		req.AddCookie(c)
	}
	return req, nil
}

// skip reports a nil value, which is left out of the request; when p is required that is an
// error.
func (b *RequestBuilder) skip(v any, p Param) bool {
	if !isNil(v) {
		return false
	}
	b.err = absent(p)
	return true
}

func (b *RequestBuilder) setBody(data []byte, mediaType string, err error) {
	if b.err != nil {
		return
	}
	if err != nil {
		b.err = err
		return
	}
	b.body, b.contentType = bytes.NewReader(data), mediaType
}

// ParseBaseURL parses the base URL of a client, which needs a scheme and a host.
func ParseBaseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBaseURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: %q needs a scheme and a host", ErrBaseURL, s)
	}
	return u, nil
}

// Send sends req with d and reads the whole body, which it closes. The response comes back with
// the body in memory, so it can be read again.
func Send(d Doer, req *http.Request) (*http.Response, []byte, error) {
	res, err := d.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if res.Body == nil {
		return res, nil, nil
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, body, nil
}

// isNil reports a nil pointer, slice, map or interface, or no value at all.
func isNil(v any) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return rv.IsNil()
	case reflect.Invalid:
		return true
	default:
		return false
	}
}

// escapeSegment percent-encodes what a path segment cannot carry, keeping the characters the
// styles write between items: , ; = . and the other sub-delimiters.
func escapeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		if isSegmentChar(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}

// isSegmentChar reports a pchar of RFC 3986: unreserved, a sub-delimiter, : or @.
func isSegmentChar(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~!$&'()*+,;=:@", c) >= 0
}
