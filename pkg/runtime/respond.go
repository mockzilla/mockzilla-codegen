// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"io"
	"net/http"
)

// Write writes a response: its headers, then body under contentType. A nil body sends the status
// alone; a File streams; bytes and strings go as they are; anything else is written as JSON.
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
	case []byte:
		return WriteBytes(w, status, b)
	case string:
		return WriteBytes(w, status, []byte(b))
	}
	return WriteJSON(w, status, body)
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
	return err
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
	return err
}
