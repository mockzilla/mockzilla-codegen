// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MediaTypeEventStream is the media type of Server-Sent Events.
const MediaTypeEventStream = "text/event-stream"

// lineMediaTypes are the media types that carry one JSON value per line.
var lineMediaTypes = []string{
	"application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines", "application/json-lines",
}

// Event is one frame of a stream: the fields of a Server-Sent Event, or, on a line-delimited
// stream, the line in Data alone. Type is the event field; Retry is the reconnection time the
// server asks for, 0 when the event names none.
type Event struct {
	ID    string
	Type  string
	Retry time.Duration
	Data  []byte
}

// Stream reads the frames of a response one at a time, as bufio.Scanner does: Next reads the
// next frame into Current, and Err reports what stopped Next. The caller owns the response and
// closes the stream. Sentinels are frames that end the stream instead of being decoded, such as
// the [DONE] some APIs send last; set them before the first Next. Canceling the context of the
// request unblocks a pending Next, and Err then reports the context's error.
type Stream[T any] struct {
	Sentinels []string

	body    io.ReadCloser
	reader  *bufio.Reader
	framer  func(*bufio.Reader) (Event, bool, error)
	current T
	event   Event
	err     error
	isDone  bool
}

// NewStream frames the body of res by its Content-Type: Server-Sent Events for
// text/event-stream, else one frame per line.
func NewStream[T any](res *http.Response) *Stream[T] {
	if ContentType(res.Header) == MediaTypeEventStream {
		return NewEventStream[T](res)
	}
	return NewLineStream[T](res)
}

// NewEventStream reads the body of res as Server-Sent Events. An event's data lines are joined
// with newlines; comments are skipped; an event without data is skipped too, after its id and
// retry are taken.
func NewEventStream[T any](res *http.Response) *Stream[T] {
	return newStream[T](res, readEvent)
}

// NewLineStream reads the body of res one line at a time, skipping empty lines.
func NewLineStream[T any](res *http.Response) *Stream[T] {
	return newStream[T](res, readLine)
}

func newStream[T any](res *http.Response, framer func(*bufio.Reader) (Event, bool, error)) *Stream[T] {
	return &Stream[T]{body: res.Body, reader: bufio.NewReader(res.Body), framer: framer}
}

// Next reads the next frame into Current and reports whether there was one. It returns false at
// the end of the stream, at a sentinel, after Close, and on an error, which Err then reports.
func (s *Stream[T]) Next() bool {
	if s.isDone {
		return false
	}

	event, ok, err := s.framer(s.reader)
	if err != nil {
		s.stop(err)
		return false
	}
	if !ok || slices.Contains(s.Sentinels, string(bytes.TrimSpace(event.Data))) {
		s.stop(nil)
		return false
	}

	var current T
	if err = decodeFrame(event.Data, &current); err != nil {
		s.stop(fmt.Errorf("%w: %w", ErrFrame, err))
		return false
	}
	s.current, s.event = current, event
	return true
}

// Current is the frame the last Next read.
func (s *Stream[T]) Current() T {
	return s.current
}

// Event is the raw frame behind Current, with the id, event and retry fields of a Server-Sent
// Event.
func (s *Stream[T]) Event() Event {
	return s.event
}

// Err is what stopped Next: nil at the end of the stream or at a sentinel, else the read or
// decode error, or the error of the request's context when it was canceled.
func (s *Stream[T]) Err() error {
	return s.err
}

// Close closes the response body. Next returns false afterwards.
func (s *Stream[T]) Close() error {
	s.isDone = true
	return s.body.Close()
}

// All iterates over the frames, delivering the error that stops the stream as the last pair.
func (s *Stream[T]) All() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for s.Next() {
			if !yield(s.current, nil) {
				return
			}
		}
		if s.err != nil {
			var zero T
			yield(zero, s.err)
		}
	}
}

func (s *Stream[T]) stop(err error) {
	s.isDone, s.err = true, err
}

// streamBody is the body of a streamed response, whose request lives until the body is closed.
type streamBody struct {
	io.ReadCloser

	cancel context.CancelCauseFunc
}

func (b *streamBody) Close() error {
	defer b.cancel(nil)
	return b.ReadCloser.Close()
}

// IsSequential reports a media type whose body is a sequence of frames: text/event-stream and
// the line-delimited JSON types, with or without parameters.
func IsSequential(mediaType string) bool {
	mediaType, _, _ = strings.Cut(strings.ToLower(mediaType), ";")
	mediaType = strings.TrimSpace(mediaType)
	return mediaType == MediaTypeEventStream || slices.Contains(lineMediaTypes, mediaType)
}

// IsStreaming reports a 2xx response with a body in a sequential media type, which SendStream
// leaves unread for a Stream.
func IsStreaming(res *http.Response) bool {
	return res.StatusCode >= 200 && res.StatusCode <= 299 && res.Body != nil && IsSequential(ContentType(res.Header))
}

// SendStream is Send for a method that streams: it asks for mediaType unless the request says
// what it accepts, and leaves the body of a response IsStreaming reports unread, with no bytes
// returned. Every other response is read whole, as Send reads it. A timeout above 0 bounds the
// call until a stream starts: the wait for its headers, or the whole call for any other response.
// Closing the body of a stream ends its request.
func SendStream(d Doer, req *http.Request, mediaType string, timeout time.Duration) (*http.Response, []byte, error) {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", mediaType)
	}

	ctx, cancel := context.WithCancelCause(req.Context())
	if timeout > 0 {
		timer := time.AfterFunc(timeout, func() { cancel(context.DeadlineExceeded) })
		defer timer.Stop()
	}

	res, err := d.Do(req.WithContext(ctx))
	if err == nil && IsStreaming(res) {
		res.Body = &streamBody{ReadCloser: res.Body, cancel: cancel}
		return res, nil, nil
	}

	defer cancel(nil)
	if err != nil {
		return nil, nil, err
	}
	return readBody(res)
}

// OpenStream returns a stream over the frames of a response SendStream returned with its body.
// A 2xx response in another media type is ErrContentType; a status outside 2xx is an *APIError,
// as DecodeSuccess reports it with targets.
func OpenStream[T any](res *http.Response, body []byte, targets []Target) (*Stream[T], error) {
	if IsStreaming(res) {
		return NewStream[T](res), nil
	}
	if res.StatusCode >= 200 && res.StatusCode <= 299 {
		return nil, ContentTypeError(ContentType(res.Header))
	}
	return nil, DecodeSuccess(res, body, targets)
}

// readEvent reads one Server-Sent Event with data, or reports the end of the body. A line that
// is not a field, such as a comment, is skipped; an event without data is not dispatched.
func readEvent(r *bufio.Reader) (Event, bool, error) {
	var e Event
	var data [][]byte
	for {
		line, err := readFrameLine(r)
		if err != nil {
			return Event{}, false, err
		}
		if line == nil {
			break
		}
		if len(line) == 0 {
			if data != nil {
				break
			}
			e.Type = ""
			continue
		}

		field, value := splitField(line)
		switch field {
		case "data":
			data = append(data, value)
		case "event":
			e.Type = string(value)
		case "id":
			e.ID = string(value)
		case "retry":
			if ms, convErr := strconv.Atoi(string(value)); convErr == nil && ms >= 0 {
				e.Retry = time.Duration(ms) * time.Millisecond
			}
		}
	}
	if data == nil {
		return Event{}, false, nil
	}
	e.Data = bytes.Join(data, []byte("\n"))
	return e, true, nil
}

// readLine reads the next line that is not empty, or reports the end of the body.
func readLine(r *bufio.Reader) (Event, bool, error) {
	for {
		line, err := readFrameLine(r)
		switch {
		case err != nil:
			return Event{}, false, err
		case line == nil:
			return Event{}, false, nil
		case len(line) > 0:
			return Event{Data: line}, true, nil
		}
	}
}

// readFrameLine reads one line without its line ending, LF or CRLF. A nil line is the end of the
// body; a line without an ending before the end is returned as it is.
func readFrameLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(line) == 0 {
		return nil, nil
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r")), nil
}

// splitField splits a Server-Sent Events line into its field name and value, dropping the one
// space the value may start with.
func splitField(line []byte) (string, []byte) {
	field, value, _ := bytes.Cut(line, []byte(":"))
	return string(field), bytes.TrimPrefix(value, []byte(" "))
}

// decodeFrame reads data into dst: as it is into bytes, and as JSON into anything else.
func decodeFrame(data []byte, dst any) error {
	if raw, ok := dst.(*[]byte); ok {
		*raw = bytes.Clone(data)
		return nil
	}
	return json.Unmarshal(data, dst)
}
