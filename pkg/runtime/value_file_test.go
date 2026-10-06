// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errRead = errors.New("read failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errRead
}

type closer struct {
	io.Reader
	isClosed bool
}

func (c *closer) Close() error {
	c.isClosed = true
	return nil
}

func multipartFile(t *testing.T) *multipart.FileHeader {
	t.Helper()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="a.txt"`)
	h.Set("Content-Type", "text/plain")
	part, err := w.CreatePart(h)
	require.NoError(t, err)
	_, err = part.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, w.Close())

	form, err := multipart.NewReader(&body, w.Boundary()).ReadForm(1 << 20)
	require.NoError(t, err)
	return form.File["file"][0]
}

func TestFile(t *testing.T) {
	t.Parallel()

	type want struct {
		name        string
		contentType string
		size        int64
		data        string
	}
	tests := []struct {
		name string
		file File
		want want
	}{
		{
			name: "Bytes",
			file: NewFile([]byte("hello"), "a.txt", "text/plain"),
			want: want{name: "a.txt", contentType: "text/plain", size: 5, data: "hello"},
		},
		{
			name: "Reader",
			file: NewFileReader(strings.NewReader("hello"), "a.bin", "application/octet-stream", -1),
			want: want{name: "a.bin", contentType: "application/octet-stream", size: -1, data: "hello"},
		},
		{
			name: "Multipart",
			file: NewFileFromMultipart(multipartFile(t)),
			want: want{name: "a.txt", contentType: "text/plain", size: 5, data: "hello"},
		},
		{
			name: "Zero value",
			want: want{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b, err := tt.file.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tt.want, want{
				name:        tt.file.Name(),
				contentType: tt.file.ContentType(),
				size:        tt.file.Size(),
				data:        string(b),
			})
		})
	}
}

func TestFileReader(t *testing.T) {
	t.Parallel()

	src := &closer{Reader: strings.NewReader("hello")}
	rc, err := NewFileReader(src, "", "", 5).Reader()
	require.NoError(t, err)
	b, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	assert.Equal(t, "hello", string(b))
	assert.True(t, src.isClosed)

	rc, err = NewFile([]byte("hi"), "", "").Reader()
	require.NoError(t, err)
	b, err = io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "hi", string(b))
}

func TestFileErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file File
	}{
		{name: "Reader fails", file: NewFileReader(failingReader{}, "", "", -1)},
		{name: "Multipart part cannot be opened", file: NewFileFromMultipart(&multipart.FileHeader{Filename: "gone"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.file.Bytes()
			require.Error(t, err)
			_, err = json.Marshal(tt.file)
			require.Error(t, err)
		})
	}
}

func TestFileJSON(t *testing.T) {
	t.Parallel()

	b, err := json.Marshal(NewFile([]byte("hello"), "a.txt", ""))
	require.NoError(t, err)
	assert.JSONEq(t, `"aGVsbG8="`, string(b))

	var f File
	require.NoError(t, json.Unmarshal(b, &f))
	assert.Equal(t, NewFile([]byte("hello"), "", ""), f)
	require.Error(t, json.Unmarshal([]byte(`5`), &f))
}
