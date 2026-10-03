// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type address struct {
	City    string `json:"city"`
	Country string `json:"country"`
}

type order struct {
	Name    string          `json:"name"`
	Age     *int            `json:"age,omitempty"`
	Active  bool            `json:"active"`
	Address address         `json:"address"`
	Items   []string        `json:"items"`
	Lines   []address       `json:"lines"`
	Extra   map[string]any  `json:"extra"`
	Phone   string          `json:"phone"`
	Codes   []int           `json:"codes"`
	Meta    map[string]bool `json:"meta"`
	Ignored string          `json:"-"`
}

type upload struct {
	Title    string   `json:"title"`
	File     File     `json:"file"`
	Optional *File    `json:"optional"`
	Files    []File   `json:"files"`
	Tags     []string `json:"tags"`
	Address  *address `json:"address"`
	Point    address  `json:"point"`
	Count    int      `json:"count"`
	hidden   string   //nolint:unused // left alone by the decoder
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestContentType(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "application/json", ContentType(http.Header{"Content-Type": {"application/json; charset=utf-8"}}))
	assert.Empty(t, ContentType(http.Header{"Content-Type": {"not/valid/type;;"}}))
	assert.True(t, IsJSON("application/vnd.api+json"))
	assert.False(t, IsJSON("text/plain"))
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       io.Reader
		isRequired bool
		want       address
		wantErr    error
	}{
		{name: "Object", body: strings.NewReader(`{"city":"Berlin"}`), want: address{City: "Berlin"}},
		{name: "Empty optional body", body: strings.NewReader(" \n")},
		{name: "Empty required body", body: strings.NewReader(""), isRequired: true, wantErr: ErrBodyEmpty},
		{name: "Broken JSON", body: strings.NewReader("{"), wantErr: io.ErrUnexpectedEOF},
		{name: "Broken reader", body: errReader{}, wantErr: io.ErrUnexpectedEOF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got address
			err := DecodeJSON(tc.body, &got, tc.isRequired)

			if tc.wantErr != nil {
				require.Error(t, err)
				if errors.Is(tc.wantErr, ErrBodyEmpty) {
					require.ErrorIs(t, err, ErrBodyEmpty)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		isRequired bool
		want       order
		wantErr    error
	}{
		{
			name: "Flat and nested keys",
			body: "name=Ann&age=30&active=true&address[city]=Berlin&address[country]=DE&items[0]=a&items[1]=b&codes[]=1&codes[]=2&meta[x]=true&ignored=1",
			want: order{Name: "Ann", Age: new(30), Active: true, Address: address{City: "Berlin", Country: "DE"}, Items: []string{"a", "b"}, Codes: []int{1, 2}, Meta: map[string]bool{"x": true}},
		},
		{
			name: "Objects in a list",
			body: "lines[0][city]=A&lines[1][city]=B",
			want: order{Lines: []address{{City: "A"}, {City: "B"}}},
		},
		{
			name: "Untyped values are read conservatively",
			body: "extra[n]=42&extra[f]=1.5&extra[b]=false&extra[zeros]=007&extra[phone]=%2B123&extra[text]=a+b&extra[list][]=1&extra[list][]=x&extra[one][]=x&extra[obj][k]=v&extra[big]=99999999999999999999",
			want: order{Extra: map[string]any{
				"n": int64(42), "f": 1.5, "b": false, "zeros": "007", "phone": "+123", "text": "a b",
				"list": []any{int64(1), "x"}, "one": []any{"x"}, "obj": map[string]any{"k": "v"}, "big": "99999999999999999999",
			}},
		},
		{name: "Phone number stays a string", body: "phone=%2B4930", want: order{Phone: "+4930"}},
		{name: "Empty optional body"},
		{name: "Empty required body", isRequired: true, wantErr: ErrBodyEmpty},
		{name: "Bad number", body: "age=x", wantErr: ErrParamValue},
		{name: "Bad query", body: "a=%zz", wantErr: url.EscapeError("%zz")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got order
			err := DecodeForm(strings.NewReader(tc.body), &got, tc.isRequired)

			if tc.wantErr != nil {
				require.Error(t, err)
				if tc.wantErr != nil && errors.Is(tc.wantErr, ErrParamValue) && tc.name != "Bad query" {
					require.ErrorIs(t, err, tc.wantErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDecodeFormEdges(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, DecodeForm(errReader{}, new(order), false), io.ErrUnexpectedEOF)
	require.ErrorIs(t, DecodeForm(strings.NewReader("a=1"), order{}, false), ErrParamValue)
}

func multipartRequest(t *testing.T, write func(w *multipart.Writer)) *http.Request {
	t.Helper()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	write(w)
	require.NoError(t, w.Close())
	r := httptest.NewRequest(http.MethodPost, "/", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}

func TestDecodeMultipart(t *testing.T) {
	t.Parallel()

	r := multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("title", "Report"))
		require.NoError(t, w.WriteField("tags", "a"))
		require.NoError(t, w.WriteField("tags", "b"))
		require.NoError(t, w.WriteField("address", `{"city":"Berlin"}`))
		require.NoError(t, w.WriteField("point[city]", "Paris"))
		require.NoError(t, w.WriteField("count", "3"))
		for _, name := range []string{"file", "optional", "files", "files"} {
			part, err := w.CreateFormFile(name, name+".txt")
			require.NoError(t, err)
			_, err = part.Write([]byte("content of " + name))
			require.NoError(t, err)
		}
	})

	var got upload
	require.NoError(t, DecodeMultipart(r, &got, 0))

	assert.Equal(t, "Report", got.Title)
	assert.Equal(t, []string{"a", "b"}, got.Tags)
	assert.Equal(t, &address{City: "Berlin"}, got.Address)
	assert.Equal(t, address{City: "Paris"}, got.Point)
	assert.Equal(t, 3, got.Count)
	assert.Equal(t, "file.txt", got.File.Name())
	data, err := got.File.Bytes()
	require.NoError(t, err)
	assert.Equal(t, "content of file", string(data))
	require.NotNil(t, got.Optional)
	assert.Equal(t, "optional.txt", got.Optional.Name())
	assert.Len(t, got.Files, 2)
}

func TestDecodeMultipartEdges(t *testing.T) {
	t.Parallel()

	empty := multipartRequest(t, func(*multipart.Writer) {})
	var got upload
	require.NoError(t, DecodeMultipart(empty, &got, 1))
	assert.Equal(t, upload{}, got)

	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(*multipart.Writer) {}), new(int), 0), ErrParamValue)
	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(*multipart.Writer) {}), 1, 0), ErrParamValue)
	require.ErrorIs(t, DecodeMultipart(multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("count", "x"))
	}), &got, 0), ErrParamValue)
	require.Error(t, DecodeMultipart(multipartRequest(t, func(w *multipart.Writer) {
		require.NoError(t, w.WriteField("address", "{"))
	}), &got, 0))

	plain := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("a=1"))
	plain.Header.Set("Content-Type", "text/plain")
	require.Error(t, DecodeMultipart(plain, &got, 0))
}

func TestDecodeTextAndBytes(t *testing.T) {
	t.Parallel()

	text, err := DecodeText(strings.NewReader("hi"), true)
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
	_, err = DecodeText(strings.NewReader(""), true)
	require.ErrorIs(t, err, ErrBodyEmpty)
	_, err = DecodeText(errReader{}, false)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	data, err := DecodeBytes(strings.NewReader("hi"), false)
	require.NoError(t, err)
	assert.Equal(t, []byte("hi"), data)
	data, err = DecodeBytes(strings.NewReader(""), false)
	require.NoError(t, err)
	assert.Nil(t, data)
	_, err = DecodeBytes(strings.NewReader(""), true)
	require.ErrorIs(t, err, ErrBodyEmpty)
}

func TestDecodeFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       io.Reader
		isRequired bool
		want       string
		wantType   string
		wantSize   int64
		wantErr    error
	}{
		{name: "A body streams under its media type", body: strings.NewReader("png"), want: "png", wantType: "image/png", wantSize: 3},
		{name: "An empty optional body", body: strings.NewReader("")},
		{name: "An empty required body", body: strings.NewReader(""), isRequired: true, wantErr: ErrBodyEmpty},
		{name: "A broken body", body: errReader{}, wantErr: io.ErrUnexpectedEOF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPut, "/", tc.body)
			r.Header.Set("Content-Type", "Image/PNG; q=1")

			f, err := DecodeFile(r, tc.isRequired)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			data, err := f.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(data))
			assert.Equal(t, tc.wantType, f.ContentType())
			assert.Equal(t, tc.wantSize, f.Size())
		})
	}
}
