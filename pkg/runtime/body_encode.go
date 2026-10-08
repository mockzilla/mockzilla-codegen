// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Writing Go values as url-encoded forms and multipart bodies.

package runtime

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

// EncodeForm writes v, a struct or a map, as form values: nested objects with bracketed keys,
// address[city]=Berlin, lists as repeated keys, tags=a&tags=b, and lists of objects with an
// index, lines[0][city]=Berlin. Values go through their JSON form, so json tags and marshalers
// apply. A value that writes its own JSON object or array, such as a union, is one JSON value. So
// is a property enc declares JSON.
func EncodeForm(v any, enc Encoding) (url.Values, error) {
	fields, err := jsonObject(v)
	if err != nil {
		return nil, err
	}

	out := url.Values{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		var mediaType string
		if mediaType, err = enc.partType(name, isFormText(fields[name])); err != nil {
			return nil, err
		}
		if IsJSON(mediaType) {
			out.Add(name, string(encodeJSON(fields[name])))
			delete(fields, name)
		}
	}
	addForm(out, "", jsonFields(reflect.ValueOf(v), fields))
	return out, nil
}

// WriteMultipart writes v, a struct, to mw as a multipart form and closes mw, which ends the
// form: File fields as file parts with their name and content type, application/octet-stream
// without one, lists as one part per item, structs and maps as JSON parts, bytes as base64, and
// everything else as text. A property enc declares a media type for is written in it. A nil slice
// or map is left out, and a zero field tagged omitempty or omitzero, as JSON leaves them out.
func WriteMultipart(mw *multipart.Writer, v any, enc Encoding) error {
	return (&formWriter{mw: mw, encoding: enc}).write(v)
}

// MultipartSize writes v as a multipart form without reading its files, and returns the boundary
// it used with the length of the form, -1 when a file does not know its size.
func MultipartSize(v any, enc Encoding) (int64, string, error) {
	var n byteCounter
	w := &formWriter{mw: multipart.NewWriter(&n), encoding: enc, isCounting: true}
	if err := w.write(v); err != nil {
		return 0, "", err
	}
	if w.isUnsized {
		return -1, w.mw.Boundary(), nil
	}
	return int64(n) + w.size, w.mw.Boundary(), nil
}

// EncodeText writes v, a scalar, as a text body carries it; another value is ErrContentType.
func EncodeText(v any) (string, error) {
	rv, ok := held(reflect.ValueOf(v))
	switch {
	case !ok:
		return "", nil
	case !isScalar(rv.Type()):
		return "", fmt.Errorf("%w: cannot write %T as text", ErrContentType, v)
	}
	return text(rv)
}

// formWriter writes the parts of a multipart form. Counting, it reads no file and adds the file
// sizes to size instead; isUnsized records a file that does not know its size.
type formWriter struct {
	mw         *multipart.Writer
	encoding   Encoding
	isCounting bool
	size       int64
	isUnsized  bool
}

func (w *formWriter) write(v any) error {
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: a multipart form needs a struct, not %T", ErrBodyValue, v)
	}
	if reflect.PointerTo(rv.Type()).Implements(formUnmarshaler) {
		return w.object(rv)
	}

	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		name := jsonName(f)
		if name == "" || !f.IsExported() || isOmitted(f, rv.Field(i)) {
			continue
		}
		if err := w.part(name, rv.Field(i)); err != nil {
			return err
		}
	}
	return w.mw.Close()
}

// part writes one field of a multipart form, a list item by item and each item by its type.
func (w *formWriter) part(name string, v reflect.Value) error {
	v, ok := held(v)
	if !ok {
		return nil
	}

	t := v.Type()
	switch {
	case t == fileType:
		f, _ := v.Interface().(File)
		return w.file(name, f)
	case (t.Kind() == reflect.Slice || t.Kind() == reflect.Map) && v.IsNil():
		return nil
	case t.Kind() == reflect.Slice && !isBytes(t):
		for i := range v.Len() {
			if err := w.part(name, v.Index(i)); err != nil {
				return err
			}
		}
		return nil
	}

	isText := isBytes(t) || isScalar(t)
	mediaType, err := w.encoding.partType(name, isText)
	if err != nil {
		return err
	}
	if (mediaType == "" && !isText) || IsJSON(mediaType) {
		return w.jsonPart(name, cmp.Or(mediaType, "application/json"), v.Interface())
	}

	s, err := text(v)
	switch {
	case err != nil:
		return err
	case mediaType == "":
		return w.mw.WriteField(name, s)
	}
	return w.dataPart(name, mediaType, []byte(s))
}

// object writes a struct with UnmarshalForm member by member from its JSON, files as file parts.
func (w *formWriter) object(rv reflect.Value) error {
	files := map[string]reflect.Value{}
	data, err := json.Marshal(withoutFiles(rv, files).Interface())
	if err != nil {
		return err
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil {
		return fmt.Errorf("%w: a multipart form needs an object, not %.20q", ErrBodyValue, data)
	}

	for _, name := range slices.Sorted(maps.Keys(members)) {
		if f, isFile := files[name]; isFile {
			err = w.part(name, f)
		} else {
			err = w.member(name, members[name])
		}
		if err != nil {
			return err
		}
	}
	return w.mw.Close()
}

// member writes one JSON member: a scalar as text, an object as a JSON part, a list item by item.
func (w *formWriter) member(name string, raw json.RawMessage) error {
	switch jsonKind(raw) {
	case KindNull:
		return nil
	case KindObject:
		return w.dataPart(name, "application/json", raw)
	case KindArray:
		var items []json.RawMessage
		_ = json.Unmarshal(raw, &items) // raw is an array of the JSON the struct wrote
		for _, item := range items {
			var err error
			if jsonKind(item) == KindArray {
				err = w.dataPart(name, "application/json", item)
			} else {
				err = w.member(name, item)
			}
			if err != nil {
				return err
			}
		}
		return nil
	default:
	}

	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	return w.mw.WriteField(name, s)
}

// jsonPart writes v as JSON in a part of its own under mediaType.
func (w *formWriter) jsonPart(name, mediaType string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.dataPart(name, mediaType, data)
}

// dataPart writes data as a part of its own under mediaType.
func (w *formWriter) dataPart(name, mediaType string, data []byte) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"`)
	h.Set("Content-Type", mediaType)
	pw, err := w.mw.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = pw.Write(data)
	return err
}

// file writes f as a file part, with its name and the content type fileType picks. A file without
// a name goes as blob, as browsers send a Blob: most servers read an empty filename as text.
func (w *formWriter) file(name string, f File) error {
	mediaType, err := w.encoding.fileType(name, f)
	if err != nil {
		return err
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+quoteEscaper.Replace(name)+`"; filename="`+quoteEscaper.Replace(cmp.Or(f.Name(), "blob"))+`"`)
	h.Set("Content-Type", mediaType)
	pw, err := w.mw.CreatePart(h)
	if err != nil {
		return err
	}
	if w.isCounting {
		w.size += f.Size()
		w.isUnsized = w.isUnsized || f.Size() < 0
		return nil
	}

	rc, err := f.Reader()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	_, err = io.Copy(pw, rc)
	return err
}

// withoutFiles copies rv, moving its files and those of the set variants of a union to files.
func withoutFiles(rv reflect.Value, files map[string]reflect.Value) reflect.Value {
	out := reflect.New(rv.Type()).Elem()
	out.Set(rv)
	for i := range rv.NumField() {
		f := rv.Type().Field(i)
		v := rv.Field(i)
		name := jsonName(f)
		switch {
		case !f.IsExported():
		case name == "" && v.Kind() == reflect.Pointer && !v.IsNil() && v.Elem().Kind() == reflect.Struct:
			variant := reflect.New(v.Elem().Type())
			variant.Elem().Set(withoutFiles(v.Elem(), files))
			out.Field(i).Set(variant)
		case name != "" && isFileField(f.Type):
			files[name] = v
			out.Field(i).Set(reflect.Zero(f.Type))
		}
	}
	return out
}

// byteCounter is a writer that counts what it is given.
type byteCounter int64

func (c *byteCounter) Write(p []byte) (int, error) {
	*c += byteCounter(len(p))
	return len(p), nil
}

// jsonObject is v as a JSON object, with numbers kept as text.
func jsonObject(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out map[string]any
	if err = dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: a form needs an object, not %.20q", ErrBodyValue, data)
	}
	return out, nil
}

// jsonFields puts the text of jsonField into encoded, the JSON of rv, for each value of rv.
func jsonFields(rv reflect.Value, encoded any) any {
	v, _ := present(rv)
	switch t := encoded.(type) {
	case map[string]any:
		if v.Kind() == reflect.Struct || v.Kind() == reflect.Map {
			for name, field := range properties(v) {
				if item, ok := t[name]; ok {
					t[name] = jsonField(field, item)
				}
			}
		}
	case []any:
		if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
			for i := range min(v.Len(), len(t)) {
				t[i] = jsonField(v.Index(i), t[i])
			}
		}
	}
	return encoded
}

// jsonField is item, the JSON of v, as text when v writes its own JSON object or array.
func jsonField(v reflect.Value, item any) any {
	v, ok := held(v)
	if !ok {
		return item
	}
	t := v.Type()
	if !t.Implements(jsonMarshaler) && !reflect.PointerTo(t).Implements(jsonMarshaler) {
		return jsonFields(v, item)
	}
	switch item.(type) {
	case map[string]any, []any:
		data, _ := json.Marshal(v.Interface()) // jsonObject has marshaled it already
		return string(data)
	}
	return item
}

// addForm adds v under key: an object with bracketed keys, a list as repeated keys, or as one
// value.
func addForm(out url.Values, key string, v any) {
	switch v := v.(type) {
	case map[string]any:
		for _, name := range slices.Sorted(maps.Keys(v)) {
			sub := name
			if key != "" {
				sub = key + "[" + name + "]"
			}
			addForm(out, sub, v[name])
		}
	case []any:
		for i, item := range v {
			switch item.(type) {
			case map[string]any, []any:
				addForm(out, key+"["+strconv.Itoa(i)+"]", item)
			default:
				addForm(out, key, item)
			}
		}
	case nil:
	default:
		out.Add(key, formText(v))
	}
}

// formText writes a JSON scalar as form text.
func formText(v any) string {
	switch v := v.(type) {
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	}
	s, _ := v.(string)
	return s
}

// isFormText reports a JSON value a form holds as text: a scalar, or a list of scalars.
func isFormText(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return false
	case []any:
		return !slices.ContainsFunc(x, isComposite)
	}
	return true
}

// isScalar reports a type written as one text: a string, a bool, a number, or one that writes
// its own text, such as time.Time.
func isScalar(t reflect.Type) bool {
	t = valueType(t)
	switch {
	case t.Implements(textMarshaler):
		return true
	case t.Kind() == reflect.Struct, t.Kind() == reflect.Map, t.Kind() == reflect.Slice, t.Kind() == reflect.Interface:
		return false
	}
	return true
}
