// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package httpclient builds and sends the requests of a generated client and reads its responses.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/validation"
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
	pathQuery   string
	query       []string
	queryString string
	header      http.Header
	cookies     []*http.Cookie
	body        io.Reader
	length      int64
	contentType string
	err         error
}

// NewRequestBuilder starts a request of method to path, a template such as /pets/{id} whose
// placeholders PathParam fills in. A query the template writes after a ? is sent as written,
// ahead of the query parameters; a fragment after a # is not sent.
func NewRequestBuilder(method, path string) *RequestBuilder {
	path, _, _ = strings.Cut(path, "#")
	path, query, _ := strings.Cut(path, "?")
	return &RequestBuilder{method: method, path: path, pathQuery: query, header: http.Header{}}
}

// PathParam fills v into the placeholder of p. A nil value is an error, since paths need every
// parameter, and so is a value written as nothing, which would send the request to another path.
func (b *RequestBuilder) PathParam(v any, p runtime.Param) {
	if b.err != nil {
		return
	}
	if isNil(v) {
		b.err = fmt.Errorf("%w: %s", runtime.ErrParamMissing, p.Name)
		return
	}

	value, err := runtime.EncodePath(v, p)
	switch {
	case err != nil:
		b.err = err
		return
	case value == "":
		b.err = fmt.Errorf("%w: %s is empty", runtime.ErrParamMissing, p.Name)
		return
	}
	b.path = strings.ReplaceAll(b.path, "{"+p.Name+"}", escape(value, isSegmentChar))
	b.pathQuery = strings.ReplaceAll(b.pathQuery, "{"+p.Name+"}", escape(value, isUnreserved))
}

// QueryParam adds v to the query as p, see runtime.EncodeQuery. An optional p is left out when v
// is nil or zero, a required one is an error when v is nil.
func (b *RequestBuilder) QueryParam(v any, p runtime.Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}
	query, err := runtime.EncodeQuery(v, p)
	b.query, b.err = append(b.query, query), err
}

// QueryString writes v as the whole query, JSON or a form; v is left out as QueryParam leaves it.
func (b *RequestBuilder) QueryString(v any, p runtime.Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}
	if p.IsJSON {
		data, err := json.Marshal(v)
		b.queryString, b.err = escape(string(data), isUnreserved), err
		return
	}
	b.queryString, b.err = runtime.EncodeForm(v, nil)
}

// HeaderParam adds v as the header p, left out as QueryParam leaves it.
func (b *RequestBuilder) HeaderParam(v any, p runtime.Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}

	value, err := runtime.EncodeHeader(v, p)
	if err != nil {
		b.err = err
		return
	}
	b.header.Add(p.Name, value)
}

// CookieParam adds v as the cookie p, left out as QueryParam leaves it. A value with a byte no
// cookie holds, such as a semicolon, is an error rather than sent altered.
func (b *RequestBuilder) CookieParam(v any, p runtime.Param) {
	if b.err != nil || b.skip(v, p) {
		return
	}

	cookies, err := runtime.EncodeCookie(v, p)
	if err != nil {
		b.err = err
		return
	}
	for _, c := range cookies {
		if err = c.Valid(); err != nil {
			b.err = fmt.Errorf("%w: %s: %w", runtime.ErrParamValue, p.Name, err)
			return
		}
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

// FormBody sends v as application/x-www-form-urlencoded with the encoding enc, see
// runtime.EncodeForm.
func (b *RequestBuilder) FormBody(v any, enc runtime.Encoding) {
	if b.err != nil {
		return
	}
	body, err := runtime.EncodeForm(v, enc)
	b.setBody([]byte(body), "application/x-www-form-urlencoded", err)
}

// MultipartBody sends v as multipart/form-data, see runtime.WriteMultipart. The form is written
// while it is sent, so its files stream; its length is known up front when every file knows its
// size.
func (b *RequestBuilder) MultipartBody(v any, enc runtime.Encoding) {
	if b.err != nil {
		return
	}
	size, boundary, err := runtime.MultipartSize(v, enc)
	if err != nil {
		b.err = err
		return
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	// The boundary MultipartSize took from another Writer, which never makes an invalid one.
	_ = mw.SetBoundary(boundary)
	b.body, b.length, b.contentType = &multipartBody{value: v, encoding: enc, writer: mw, pr: pr, pw: pw}, size, mw.FormDataContentType()
}

// TextBody sends s as it is under mediaType.
func (b *RequestBuilder) TextBody(s, mediaType string) {
	b.setBody([]byte(s), mediaType, nil)
}

// TextValueBody sends v, a number, a boolean or a value that marshals to text, as its text under mediaType.
func (b *RequestBuilder) TextValueBody(v any, mediaType string) {
	if b.err != nil {
		return
	}
	text, err := runtime.EncodeText(v)
	b.setBody([]byte(text), mediaType, err)
}

// BytesBody sends data as it is under mediaType.
func (b *RequestBuilder) BytesBody(data []byte, mediaType string) {
	b.setBody(data, mediaType, nil)
}

// FileBody streams f under mediaType, or under the file's own content type when mediaType is
// empty, with its size as the content length when it is known.
func (b *RequestBuilder) FileBody(f runtime.File, mediaType string) {
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

// Build makes the request against base: the path goes after the base's, the query is the base's,
// then the template's, the query parameters in the order they were added and the querystring, and
// a File body streams. The fragment of base is not sent. A placeholder no PathParam filled is
// missing.
func (b *RequestBuilder) Build(ctx context.Context, base *url.URL) (*http.Request, error) {
	if b.err != nil {
		return nil, b.err
	}
	for _, template := range []string{b.path, b.pathQuery} {
		if start := strings.IndexByte(template, '{'); start >= 0 {
			name, _, _ := strings.Cut(template[start+1:], "}")
			return nil, fmt.Errorf("%w: %s", runtime.ErrParamMissing, name)
		}
	}

	u := *base
	u.RawPath = strings.TrimSuffix(base.EscapedPath(), "/") + b.path
	path, err := url.PathUnescape(u.RawPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", runtime.ErrParamValue, err)
	}

	query := slices.Concat([]string{escape(base.RawQuery, isQueryChar), escape(b.pathQuery, isQueryChar)}, b.query, []string{b.queryString})
	u.Path, u.RawQuery = path, strings.Join(slices.DeleteFunc(query, func(s string) bool { return s == "" }), "&")
	u.ForceQuery, u.Fragment, u.RawFragment = false, "", ""

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

// skip reports a value left out: nil, an error when p is required, or zero when p is optional.
func (b *RequestBuilder) skip(v any, p runtime.Param) bool {
	if !isNil(v) {
		return !p.IsRequired && validation.IsZero(v)
	}
	if p.IsRequired {
		b.err = fmt.Errorf("%w: %s", runtime.ErrParamMissing, p.Name)
	}
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

// multipartBody is a multipart form written as it is read: the first Read starts writing value
// from another goroutine, and Close stops it. A body never read starts nothing.
type multipartBody struct {
	value    any
	encoding runtime.Encoding
	writer   *multipart.Writer
	pr       *io.PipeReader
	pw       *io.PipeWriter
	started  sync.Once
}

func (m *multipartBody) Read(p []byte) (int, error) {
	m.started.Do(func() {
		go func() { _ = m.pw.CloseWithError(runtime.WriteMultipart(m.writer, m.value, m.encoding)) }()
	})
	return m.pr.Read(p)
}

func (m *multipartBody) Close() error {
	return m.pr.Close()
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

// Send sends req with d and reads the whole body, which it closes. A timeout above 0 bounds the
// whole call, the body read included. The response comes back with the body in memory, so it can
// be read again. It asks for accept unless the request says what it accepts.
func Send(d Doer, req *http.Request, accept string, timeout time.Duration) (*http.Response, []byte, error) {
	if accept != "" && req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", accept)
	}
	if timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), timeout)
		defer cancel()
		req = req.WithContext(ctx)
	}

	res, err := d.Do(req)
	if err != nil {
		return nil, nil, err
	}
	return readBody(res)
}

// readBody reads the whole body of res, which it closes, and puts it back in memory.
func readBody(res *http.Response) (*http.Response, []byte, error) {
	if res.Body == nil {
		return res, nil, nil
	}

	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
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

// escape percent-encodes every byte of s that keep turns away.
func escape(s string, keep func(byte) bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		if keep(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperHex[c>>4])
		b.WriteByte(upperHex[c&15])
	}
	return b.String()
}

// isSegmentChar reports a pchar of RFC 3986: unreserved, a sub-delimiter, : or @. The
// sub-delimiters stay, since the styles write , ; = . between items.
func isSegmentChar(c byte) bool {
	return isUnreserved(c) || strings.IndexByte("!$&'()*+,;=:@", c) >= 0
}

// isUnreserved reports a byte RFC 3986 never escapes: a letter, a digit, or one of -._~.
func isUnreserved(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	}
	return strings.IndexByte("-._~", c) >= 0
}

// isQueryChar reports what a query of RFC 3986 holds as it is: a pchar, / or ?, and the % of an
// escape the spec wrote.
func isQueryChar(c byte) bool {
	return isSegmentChar(c) || strings.IndexByte("/?%", c) >= 0
}
