// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
)

// File is binary content from bytes, a reader or a multipart form. A reader can be read once.
type File struct {
	data        []byte
	reader      io.Reader
	header      *multipart.FileHeader
	name        string
	contentType string
	size        int64
}

func NewFile(data []byte, name, contentType string) File {
	return File{data: data, name: name, contentType: contentType, size: int64(len(data))}
}

// NewFileReader wraps r; size is -1 when unknown.
func NewFileReader(r io.Reader, name, contentType string, size int64) File {
	return File{reader: r, name: name, contentType: contentType, size: size}
}

func NewFileFromMultipart(h *multipart.FileHeader) File {
	return File{header: h, name: h.Filename, contentType: h.Header.Get("Content-Type"), size: h.Size}
}

func (f File) Name() string {
	return f.name
}

func (f File) ContentType() string {
	return f.contentType
}

// Size is -1 when a reader was given without one.
func (f File) Size() int64 {
	return f.size
}

func (f File) Reader() (io.ReadCloser, error) {
	switch {
	case f.header != nil:
		return f.header.Open()
	case f.reader != nil:
		if rc, ok := f.reader.(io.ReadCloser); ok {
			return rc, nil
		}
		return io.NopCloser(f.reader), nil
	}
	return io.NopCloser(bytes.NewReader(f.data)), nil
}

func (f File) Bytes() ([]byte, error) {
	if f.header == nil && f.reader == nil {
		return f.data, nil
	}

	rc, err := f.Reader()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// MarshalJSON writes the content as base64, like encoding/json does for []byte.
func (f File) MarshalJSON() ([]byte, error) {
	b, err := f.Bytes()
	if err != nil {
		return nil, err
	}
	return json.Marshal(b)
}

func (f *File) UnmarshalJSON(b []byte) error {
	var data []byte
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	*f = NewFile(data, "", "")
	return nil
}
