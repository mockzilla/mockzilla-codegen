// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
)

var lineBreaks = strings.NewReplacer("\r\n", "\n", "\r", "\n")

// Write writes a response: its headers, then body by the media type of the Content-Type it ends
// up with. A nil body sends the status alone and a File streams as it is. Under a JSON media type
// the body is JSON, a string too; under any other a string or bytes go as they are, and without a
// media type anything else is JSON. A nil pointer sends the status alone. A form goes through
// EncodeForm, multipart/form-data through WriteMultipart, a sequential media type one frame per
// value of an iter.Seq, or one frame for any other value, and a scalar under text/* as its text.
// Any other body is an ErrContentType. An error once the status is out wraps ErrResponseCut.
func Write(w http.ResponseWriter, status int, headers http.Header, body any) error {
	for key, values := range headers {
		w.Header()[key] = values
	}

	switch b := body.(type) {
	case nil:
		w.WriteHeader(status)
		return nil
	case File:
		return WriteFile(w, status, b)
	case *File:
		return WriteFile(w, status, *b)
	}

	mediaType := ContentType(w.Header())
	if IsJSON(mediaType) {
		return WriteJSON(w, status, body)
	}
	if data, ok := rawBody(body); ok {
		return WriteBytes(w, status, data)
	}
	v := reflect.ValueOf(body)
	switch {
	case mediaType == "":
		return WriteJSON(w, status, body)
	case v.Kind() == reflect.Pointer && v.IsNil():
		w.WriteHeader(status)
		return nil
	case mediaType == "application/x-www-form-urlencoded":
		return writeForm(w, status, body)
	case mediaType == "multipart/form-data":
		return writeMultipart(w, status, body)
	case IsSequential(mediaType):
		return writeFrames(w, status, mediaType, body)
	case strings.HasPrefix(mediaType, "text/") && isScalar(v.Type()):
		return writeText(w, status, v)
	}
	return fmt.Errorf("%w: cannot write %T as %s", ErrContentType, body, mediaType)
}

// WriteJSON writes body as JSON, with the content type application/json unless one is set.
func WriteJSON(w http.ResponseWriter, status int, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	return WriteBytes(w, status, data)
}

// WriteBytes writes data as it is, with the content type application/octet-stream unless one is set.
func WriteBytes(w http.ResponseWriter, status int, data []byte) error {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(status)
	_, err := w.Write(data)
	return cut(err)
}

// WriteFile streams f, with its content type unless one is set.
func WriteFile(w http.ResponseWriter, status int, f File) error {
	if w.Header().Get("Content-Type") == "" && f.ContentType() != "" {
		w.Header().Set("Content-Type", f.ContentType())
	}
	rc, err := f.Reader()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	w.WriteHeader(status)
	_, err = io.Copy(w, rc)
	return cut(err)
}

// rawBody is the text of a string or bytes, behind pointers or not.
func rawBody(body any) ([]byte, bool) {
	v := reflect.ValueOf(body)
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	switch {
	case v.Kind() == reflect.String:
		return []byte(v.String()), true
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8:
		return v.Bytes(), true
	}
	return nil, false
}

func writeText(w http.ResponseWriter, status int, v reflect.Value) error {
	for v.Kind() == reflect.Pointer && !v.IsNil() {
		v = v.Elem()
	}
	s, err := text(v)
	if err != nil {
		return err
	}
	return WriteBytes(w, status, []byte(s))
}

func writeForm(w http.ResponseWriter, status int, body any) error {
	values, err := EncodeForm(body)
	if err != nil {
		return err
	}
	return WriteBytes(w, status, []byte(values.Encode()))
}

// writeMultipart writes body as a multipart form under a boundary of its own. It writes the form
// once without its files first, so a body that is no form fails before the status goes out.
func writeMultipart(w http.ResponseWriter, status int, body any) error {
	if _, _, err := multipartSize(body); err != nil {
		return err
	}
	mw := multipart.NewWriter(w)
	w.Header().Set("Content-Type", mw.FormDataContentType())
	w.WriteHeader(status)
	return cut(WriteMultipart(mw, body))
}

// writeFrames writes the frames of body, flushing after the status and after each frame. Under
// text/event-stream each line of a frame is a data line and a blank line ends the frame; under
// any other sequential media type a frame is one line.
func writeFrames(w http.ResponseWriter, status int, mediaType string, body any) error {
	rc := http.NewResponseController(w)
	w.WriteHeader(status)
	if err := flush(rc); err != nil {
		return err
	}

	isEvents := mediaType == MediaTypeEventStream
	for frame := range framesOf(body) {
		data, ok := rawBody(frame)
		if !ok {
			var err error
			if data, err = json.Marshal(frame); err != nil {
				return cut(err)
			}
		}

		var buf bytes.Buffer
		if isEvents {
			for line := range strings.SplitSeq(lineBreaks.Replace(string(data)), "\n") {
				buf.WriteString("data: " + line + "\n")
			}
		} else {
			buf.Write(data)
		}
		buf.WriteByte('\n')
		if _, err := w.Write(buf.Bytes()); err != nil {
			return cut(err)
		}
		if err := flush(rc); err != nil {
			return err
		}
	}
	return nil
}

// framesOf is the values of body when it is an iter.Seq, none for a nil one, else body alone.
func framesOf(body any) iter.Seq[any] {
	v := reflect.ValueOf(body)
	if v.Kind() != reflect.Func || !v.Type().CanSeq() {
		return func(yield func(any) bool) { yield(body) }
	}
	return func(yield func(any) bool) {
		if v.IsNil() {
			return
		}
		for item := range v.Seq() {
			if !yield(item.Interface()) {
				return
			}
		}
	}
}

// flush sends what is written so far, unless the writer cannot flush.
func flush(rc *http.ResponseController) error {
	if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return cut(err)
	}
	return nil
}

// cut marks an error that happens once the status is out, which no error response can follow.
func cut(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrResponseCut, err)
}
