// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Decoding bodies into Go values by media type: JSON, forms, multipart, text, bytes and files.

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

// mediaTypeEventStream is the media type of Server-Sent Events.
const mediaTypeEventStream = "text/event-stream"

var fileType = reflect.TypeFor[File]()

// lineMediaTypes are the media types that carry one JSON value per line.
var lineMediaTypes = []string{
	"application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines", "application/json-lines",
}

// FormUnmarshaler is a type that reads a form itself, as a generated union in a form body does.
type FormUnmarshaler interface {
	UnmarshalForm(form *multipart.Form) error
}

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

// IsSequential reports a media type whose body is a sequence of frames: text/event-stream and
// the line-delimited JSON types, with or without parameters.
func IsSequential(mediaType string) bool {
	mediaType = baseMediaType(mediaType)
	return mediaType == mediaTypeEventStream || slices.Contains(lineMediaTypes, mediaType)
}

// ContentTypeError is the error of a body in a media type the operation does not take.
func ContentTypeError(mediaType string) error {
	return fmt.Errorf("%w: %s", ErrContentType, mediaType)
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
// number or a boolean becomes one. A field enc declares JSON is read as JSON.
func DecodeForm(body io.Reader, dst any, isRequired bool, enc Encoding) error {
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
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	return invalid(ErrBodyValue, fillForm(target, &multipart.Form{Value: values}, enc))
}

// DecodeMultipart decodes a multipart/form-data body into dst, a pointer to a struct. Fields of
// type File, *File or []File take the files of their name; a part that holds JSON fills a field of
// a struct, slice or map type; every other part is read as form text. maxMemory is how much stays
// in memory, DefaultMultipartMemory when 0. A type with UnmarshalForm reads the form itself. A part
// enc declares JSON is read as JSON.
func DecodeMultipart(r *http.Request, dst any, maxMemory int64, enc Encoding) error {
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
		return fmt.Errorf("%w: a multipart form needs a struct, not %s", ErrBodyValue, target.Type())
	}
	return invalid(ErrBodyValue, fillForm(target, r.MultipartForm, enc))
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

// fillPointer stores form in what dst points to, as fillForm does.
func fillPointer(form *multipart.Form, dst any) error {
	target, err := pointer(dst)
	if err != nil {
		return err
	}
	return fillForm(target, form, nil)
}

// fillForm stores form in target: by UnmarshalForm, field by field into a struct, else by keys.
func fillForm(target reflect.Value, form *multipart.Form, enc Encoding) error {
	target = allocate(target)
	if u, ok := target.Addr().Interface().(FormUnmarshaler); ok {
		return u.UnmarshalForm(form)
	}

	values := maps.Clone(form.Value)
	if target.Kind() == reflect.Struct {
		for i := range target.NumField() {
			f := target.Type().Field(i)
			name := jsonName(f)
			if name == "" || !f.IsExported() {
				continue
			}
			if err := fillField(target.Field(i), name, form, enc); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			delete(values, name)
		}
	}
	return assigner{isLoose: true}.assign(target, listsOf(formTree(values)))
}

// fillField stores the parts of name in field: its files, else its texts, file parts included.
func fillField(field reflect.Value, name string, form *multipart.Form, enc Encoding) error {
	if len(form.Value[name]) > 0 || len(form.File[name]) > 0 {
		if target, ok := targetOf(field); ok {
			field = target
		}
	}
	isSet, err := setFiles(field, form.File[name])
	if err != nil || isSet {
		return err
	}
	texts := slices.Clone(form.Value[name])
	for _, h := range form.File[name] {
		var data []byte
		if data, err = NewFileFromMultipart(h).Bytes(); err != nil {
			return err
		}
		texts = append(texts, string(data))
	}

	if enc.isJSON(name) {
		return setJSON(field, texts)
	}
	return setPart(field, texts)
}

// formTree nests form values by the names in their keys, with lists where the names count 0, 1, 2.
func formTree(values url.Values) map[string]any {
	fields := map[string]any{}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		setPath(fields, splitBrackets(key), values[key])
	}
	for name, v := range fields {
		fields[name] = listsOf(v)
	}
	return fields
}

// formValues writes what formTree nested back as form values, with keys relative to it.
func formValues(nested map[string]any) url.Values {
	out := url.Values{}
	for name, v := range nested {
		addValues(out, name, v)
	}
	return out
}

func addValues(out url.Values, key string, v any) {
	switch v := v.(type) {
	case string:
		out.Add(key, v)
	case []string:
		out[key] = append(out[key], v...)
	case map[string]any:
		for name, item := range v {
			addValues(out, key+"["+name+"]", item)
		}
	case []any:
		for i, item := range v {
			addValues(out, key+"["+strconv.Itoa(i)+"]", item)
		}
	}
}

// formName is the name a form key starts with: address of address[city].
func formName(key string) string {
	if path := splitBrackets(key); len(path) > 0 {
		return path[0]
	}
	return ""
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

// setPart stores the text parts of one field. A JSON object or array fills a struct, map or list.
func setPart(field reflect.Value, texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	field = allocate(field)
	kind := field.Kind()
	trimmed := []byte(strings.TrimSpace(texts[0]))
	if ((kind == reflect.Struct || kind == reflect.Map) && bytes.HasPrefix(trimmed, []byte("{")) ||
		kind == reflect.Slice && !isBytes(field.Type()) && bytes.HasPrefix(trimmed, []byte("["))) && json.Valid(trimmed) {
		return assigner{}.json(field, string(trimmed))
	}
	return assigner{isLoose: true}.assign(field, texts)
}

// setJSON reads each text as JSON: into a list an array whole and any other value as one item.
func setJSON(field reflect.Value, texts []string) error {
	if len(texts) == 0 {
		return nil
	}
	t := field.Type()
	if t.Kind() != reflect.Slice || isBytes(t) {
		return json.Unmarshal([]byte(texts[0]), field.Addr().Interface())
	}

	for _, s := range texts {
		list := reflect.New(t)
		if json.Unmarshal([]byte(s), list.Interface()) == nil {
			field.Set(reflect.AppendSlice(field, list.Elem()))
			continue
		}
		item := reflect.New(t.Elem())
		if err := json.Unmarshal([]byte(s), item.Interface()); err != nil {
			return err
		}
		field.Set(reflect.Append(field, item.Elem()))
	}
	return nil
}

// setFiles fills a File field, or bytes from a file part, and reports whether it was one.
func setFiles(field reflect.Value, headers []*multipart.FileHeader) (bool, error) {
	t := field.Type()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case isFileField(field.Type()):
	case isBytes(t) && len(headers) > 0:
		data, err := NewFileFromMultipart(headers[0]).Bytes()
		if err != nil {
			return true, err
		}
		allocate(field).SetBytes(data)
		return true, nil
	default:
		return false, nil
	}

	if len(headers) == 0 {
		return true, nil
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
	return true, nil
}

func isFileField(t reflect.Type) bool {
	t = valueType(t)
	for t.Kind() == reflect.Slice {
		t = valueType(t.Elem())
	}
	return t == fileType
}

func empty(isRequired bool) error {
	if isRequired {
		return ErrBodyEmpty
	}
	return nil
}
