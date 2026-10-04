// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// DefaultMultipartMemory is how much of a multipart form stays in memory before parts spill to disk.
const DefaultMultipartMemory = 32 << 20

var fileType = reflect.TypeFor[File]()

// ContentType is the media type of a request or response body, without its parameters. A
// parameter that does not parse, such as a charset without a value, leaves the media type.
func ContentType(h http.Header) string {
	mediaType, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil && !errors.Is(err, mime.ErrInvalidMediaParameter) {
		return ""
	}
	return mediaType
}

// IsJSON reports a JSON media type: application/json or one with a +json suffix.
func IsJSON(mediaType string) bool {
	mediaType = baseMediaType(mediaType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// DecodeJSON decodes a JSON body into dst, a pointer. An empty body is an error when the body is
// required, and leaves dst as it is otherwise.
func DecodeJSON(body io.Reader, dst any, isRequired bool) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return empty(isRequired)
	}
	return json.Unmarshal(data, dst)
}

// DecodeForm decodes an application/x-www-form-urlencoded body into dst, a pointer. Keys with
// brackets nest: address[city]=Berlin, items[0]=a. Into an untyped target, a value that reads as a
// number or a boolean becomes one.
func DecodeForm(body io.Reader, dst any, isRequired bool) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return empty(isRequired)
	}
	values, err := url.ParseQuery(string(data))
	if err != nil {
		return err
	}
	return assignForm(values, dst)
}

// DecodeMultipart decodes a multipart/form-data body into dst, a pointer to a struct. Fields of
// type File, *File or []File take the files of their name; a part that holds JSON fills a field of
// a struct, slice or map type; every other part is read as form text. maxMemory is how much stays
// in memory, DefaultMultipartMemory when 0.
func DecodeMultipart(r *http.Request, dst any, maxMemory int64) error {
	if maxMemory <= 0 {
		maxMemory = DefaultMultipartMemory
	}
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return err
	}

	target, err := pointer(dst)
	if err != nil {
		return err
	}
	if target.Kind() != reflect.Struct {
		return fmt.Errorf("%w: a multipart form needs a struct, not %s", ErrParamValue, target.Type())
	}
	form := r.MultipartForm
	values := url.Values(form.Value)
	for i := range target.NumField() {
		f := target.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() {
			continue
		}
		if isFileField(f.Type) {
			setFiles(target.Field(i), form.File[name])
			continue
		}
		if err = setPart(target.Field(i), values[name]); err != nil {
			return err
		}
		delete(values, name)
	}
	return assignForm(values, dst)
}

// DecodeText reads a text body.
func DecodeText(body io.Reader, isRequired bool) (string, error) {
	data, err := io.ReadAll(body)
	if err != nil || len(data) > 0 {
		return string(data), err
	}
	return "", empty(isRequired)
}

// DecodeBytes reads a body as it is.
func DecodeBytes(body io.Reader, isRequired bool) ([]byte, error) {
	data, err := io.ReadAll(body)
	if err != nil || len(data) > 0 {
		return data, err
	}
	return nil, empty(isRequired)
}

// DecodeFile hands the body of r to a File that streams it, under the request's media type and
// with its content length as the size. It reads one byte ahead to tell an empty body.
func DecodeFile(r *http.Request, isRequired bool) (File, error) {
	body := bufio.NewReader(r.Body)
	_, err := body.Peek(1)
	switch {
	case errors.Is(err, io.EOF):
		return File{}, empty(isRequired)
	case err != nil:
		return File{}, err
	}
	return NewFileReader(body, "", ContentType(r.Header), r.ContentLength), nil
}

func baseMediaType(mediaType string) string {
	mediaType, _, _ = strings.Cut(strings.ToLower(mediaType), ";")
	return strings.TrimSpace(mediaType)
}

// assignForm stores form values in dst, nesting bracketed keys.
func assignForm(values url.Values, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	fields := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		setPath(fields, splitBrackets(key), values[key])
	}
	return assigner{isLoose: true}.assign(target, listsOf(fields))
}

// listsOf turns objects whose keys are 0, 1, 2... or one empty key into lists, at every level.
func listsOf(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	for key, item := range m {
		m[key] = listsOf(item)
	}

	if item, isBare := m[""]; isBare && len(m) == 1 {
		if list, isList := item.([]string); isList {
			return list
		}
		return []any{item}
	}
	items := make([]any, 0, len(m))
	for i := range len(m) {
		item, isIndexed := m[strconv.Itoa(i)]
		if !isIndexed {
			return m
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return m
	}
	return items
}

// setPart stores the text parts of one field. Text that holds JSON fills a struct, slice or map.
func setPart(field reflect.Value, texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	kind := field.Type().Kind()
	for kind == reflect.Pointer {
		kind = field.Type().Elem().Kind()
		field.Set(reflect.New(field.Type().Elem()))
		field = field.Elem()
	}
	if trimmed := strings.TrimSpace(texts[0]); (kind == reflect.Struct || kind == reflect.Map) && strings.HasPrefix(trimmed, "{") ||
		kind == reflect.Slice && strings.HasPrefix(trimmed, "[") {
		return json.Unmarshal([]byte(trimmed), field.Addr().Interface())
	}
	return assigner{isLoose: true}.assign(field, texts)
}

func setFiles(field reflect.Value, headers []*multipart.FileHeader) {
	if len(headers) == 0 {
		return
	}
	switch field.Kind() {
	case reflect.Slice:
		files := make([]File, len(headers))
		for i, h := range headers {
			files[i] = NewFileFromMultipart(h)
		}
		field.Set(reflect.ValueOf(files))
	case reflect.Pointer:
		field.Set(reflect.ValueOf(Ptr(NewFileFromMultipart(headers[0]))))
	default:
		field.Set(reflect.ValueOf(NewFileFromMultipart(headers[0])))
	}
}

func isFileField(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t == fileType
}

func empty(isRequired bool) error {
	if isRequired {
		return ErrBodyEmpty
	}
	return nil
}
