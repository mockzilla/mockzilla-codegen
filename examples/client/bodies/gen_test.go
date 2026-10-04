// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bodies

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func mediaType(r *http.Request) string {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return mt
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeText(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, s)
}

func postJSON(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength != 0 && mediaType(r) != "application/json" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	var note *Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, note)
}

func postForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	note := Note{Text: r.PostForm.Get("text")}
	if v := r.PostForm.Get("stars"); v != "" {
		stars, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		note.Stars = &stars
	}
	writeJSON(w, note)
}

func upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var meta map[string]any
	if err = json.Unmarshal([]byte(r.PostFormValue("meta")), &meta); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{
		"title": r.PostFormValue("title"),
		"file":  string(content),
		"name":  header.Filename,
		"type":  header.Header.Get("Content-Type"),
		"tags":  r.PostForm["tags"],
		"meta":  meta,
	})
}

func postText(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch mediaType(r) {
	case "":
		writeText(w, "nothing")
	case "text/plain":
		writeText(w, "text: "+string(body))
	case "application/octet-stream":
		writeText(w, "bytes: "+string(body))
	default:
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func putFile(w http.ResponseWriter, r *http.Request) {
	if mediaType(r) != "image/png" {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(body)
}

func postAny(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch mediaType(r) {
	case "application/xml":
		writeText(w, "xml: "+string(body))
	case "text/xml":
		writeText(w, "text xml: "+string(body))
	case "application/octet-stream":
		writeText(w, "any: "+string(body))
	default:
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func getAny(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"type":"A+"}`)
}

func newClient(t *testing.T) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /json", postJSON)
	mux.HandleFunc("POST /form", postForm)
	mux.HandleFunc("POST /upload", upload)
	mux.HandleFunc("POST /text", postText)
	mux.HandleFunc("PUT /file", putFile)
	mux.HandleFunc("POST /any", postAny)
	mux.HandleFunc("GET /any/text", getAny)
	mux.HandleFunc("GET /any/bytes", getAny)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestBodies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name string
		call func() (any, error)
		want any
	}{
		{
			name: "JSON",
			call: func() (any, error) {
				return c.PostJSON(ctx, &PostJSONRequestOptions{Body: &Note{Text: "hi", Stars: new(3)}})
			},
			want: &Note{Text: "hi", Stars: new(3)},
		},
		{
			name: "An optional body left out",
			call: func() (any, error) { return c.PostJSON(ctx, nil) },
			want: (*Note)(nil),
		},
		{
			name: "A form",
			call: func() (any, error) {
				return c.PostForm(ctx, &PostFormRequestOptions{Body: &Note{Text: "hi", Stars: new(2)}})
			},
			want: &Note{Text: "hi", Stars: new(2)},
		},
		{
			name: "Text",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{BodyText: new("hello")}) },
			want: new("text: hello"),
		},
		{
			name: "Bytes",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{BodyOctetStream: []byte{0, 1}}) },
			want: new("bytes: \x00\x01"),
		},
		{
			name: "No body",
			call: func() (any, error) { return c.PostText(ctx, &PostTextRequestOptions{}) },
			want: new("nothing"),
		},
		{
			name: "The first of two media types with one tag",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyXML: new("<a/>")}) },
			want: new("xml: <a/>"),
		},
		{
			name: "The second of them",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyTextXML: new("<a/>")}) },
			want: new("text xml: <a/>"),
		},
		{
			name: "A wildcard sends bytes as an octet stream",
			call: func() (any, error) { return c.PostAny(ctx, &PostAnyRequestOptions{BodyAny: []byte("png")}) },
			want: new("any: png"),
		},
		{
			name: "A wildcard string takes a JSON answer as it came",
			call: func() (any, error) { return c.GetAnyText(ctx, nil) },
			want: new(`{"type":"A+"}`),
		},
		{
			name: "Wildcard bytes take a JSON answer as it came",
			call: func() (any, error) { return c.GetAnyBytes(ctx, nil) },
			want: []byte(`{"type":"A+"}`),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.call()

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMultipart(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	body := &UploadRequestBody{
		Title: new("Cat"),
		File:  new(runtime.NewFile([]byte("meow"), "cat.txt", "text/plain")),
		Tags:  []string{"a", "b"},
		Meta:  &Note{Text: "inner"},
	}

	got, err := c.Upload(context.Background(), &UploadRequestOptions{Body: body})

	require.NoError(t, err)
	assert.Equal(t, UploadResponse200{
		"title": "Cat", "file": "meow", "name": "cat.txt", "type": "text/plain",
		"tags": []any{"a", "b"}, "meta": map[string]any{"text": "inner"},
	}, got)
}

func TestFile(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	png := runtime.NewFile([]byte("\x89PNG"), "cat.png", "image/png")

	got, err := c.PutFile(context.Background(), &PutFileRequestOptions{Body: &png})

	require.NoError(t, err)
	data, err := got.Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("\x89PNG"), data)
	assert.Equal(t, "image/png", got.ContentType())
}

func TestRequiredBody(t *testing.T) {
	t.Parallel()

	c := newClient(t)

	_, err := c.PostForm(context.Background(), &PostFormRequestOptions{})

	require.ErrorIs(t, err, runtime.ErrBodyEmpty)
}

func TestBodyTheClientCannotSend(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	tests := []struct {
		name string
		opts *PutXMLRequestOptions
		want error
	}{
		{name: "XML into a struct", opts: &PutXMLRequestOptions{Body: &Note{Text: "hi"}}, want: runtime.ErrContentType},
		{name: "No body", opts: &PutXMLRequestOptions{}, want: runtime.ErrBodyEmpty},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := c.PutXML(context.Background(), tc.opts)

			require.ErrorIs(t, err, tc.want)
		})
	}
}
