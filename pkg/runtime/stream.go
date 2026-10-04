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
	"math"
	"net/http"
	"slices"
	"strconv"
	"sync/atomic"
	"time"
)

// MediaTypeEventStream is the media type of Server-Sent Events.
const MediaTypeEventStream = "text/event-stream"

const byteOrderMark = "\xEF\xBB\xBF"

// lineMediaTypes are the media types that carry one JSON value per line.
var lineMediaTypes = []string{
	"application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines", "application/json-lines",
}

// Event is one frame of a stream: the fields of a Server-Sent Event, or, on a line-delimited
// stream, the line in Data alone. ID is the last event ID, which stays from one event to the next
// until an id field changes it; Type is the event field; Retry is the reconnection time the server
// asks for, 0 when the event names none.
type Event struct {
	ID    string
	Type  string
	Retry time.Duration
	Data  []byte
}

// Stream reads the frames of a response one at a time, as bufio.Scanner does: Next reads the
// next frame into Current, and Err reports what stopped Next. The caller owns the response and
// closes the stream. Sentinels are frames that end the stream instead of being decoded, such as
// the [DONE] some APIs send last; MaxFrameSize is the most bytes one line of the body, or the
// data of one event, may hold, 0 for no limit. Set both before the first Next. Canceling the
// context of the request unblocks a pending Next, and Err then reports the context's error.
// Close, called from another goroutine, unblocks it too, and Err then reports nil.
type Stream[T any] struct {
	Sentinels    []string
	MaxFrameSize int

	body     io.ReadCloser
	frame    func(limit int) (Event, bool, error)
	current  T
	event    Event
	err      error
	isDone   bool
	isClosed atomic.Bool
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
// retry are taken. A line ends in LF, CRLF or a lone CR, and a byte order mark at the start of the
// body is dropped. An event with data that the body ends before its blank line is not delivered,
// and Err reports io.ErrUnexpectedEOF.
func NewEventStream[T any](res *http.Response) *Stream[T] {
	body := bodyOf(res)
	events := &eventReader{lines: bufio.NewReader(&lfReader{reader: body})}
	return &Stream[T]{body: body, frame: events.next}
}

// NewLineStream reads the body of res one line at a time, skipping lines that are empty or hold
// only spaces.
func NewLineStream[T any](res *http.Response) *Stream[T] {
	body := bodyOf(res)
	lines := bufio.NewReader(body)
	return &Stream[T]{body: body, frame: func(limit int) (Event, bool, error) { return readLine(lines, limit) }}
}

// Next reads the next frame into Current and reports whether there was one. It returns false at
// the end of the stream, at a sentinel, after Close, and on an error, which Err then reports. A
// stream of anything but []byte skips an event whose data is empty, which no type decodes from.
func (s *Stream[T]) Next() bool {
	if s.isDone {
		return false
	}

	for {
		event, ok, err := s.frame(s.MaxFrameSize)
		if s.isClosed.Load() {
			s.stop(nil)
			return false
		}
		if err != nil {
			s.stop(err)
			return false
		}
		data := bytes.TrimSpace(event.Data)
		if !ok || slices.Contains(s.Sentinels, string(data)) {
			s.stop(nil)
			return false
		}

		var current T
		if _, isRaw := any(&current).(*[]byte); !isRaw && len(data) == 0 {
			continue
		}
		if err = decodeFrame(event.Data, &current); err != nil {
			s.stop(fmt.Errorf("%w: %w", ErrFrame, err))
			return false
		}
		s.current, s.event = current, event
		return true
	}
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

// Err is what stopped Next: nil at the end of the stream, at a sentinel or after Close, else the
// read or decode error, io.ErrUnexpectedEOF when an event stream ends inside an event, or the
// error of the request's context when it was canceled.
func (s *Stream[T]) Err() error {
	return s.err
}

// Close closes the response body. Next returns false afterwards.
func (s *Stream[T]) Close() error {
	s.isClosed.Store(true)
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

// eventReader reads Server-Sent Events from lines that end in LF.
type eventReader struct {
	lines     *bufio.Reader
	lastID    string
	isStarted bool
}

// next reads one event with data, or reports the end of the body. A line that is not a field,
// such as a comment, is skipped; an event without data is not dispatched. The end of the body
// inside an event with data is io.ErrUnexpectedEOF; a line or data longer than limit, when it is
// above 0, is ErrFrameSize.
func (r *eventReader) next(limit int) (Event, bool, error) {
	var e Event
	var data [][]byte
	size := 0
	for {
		line, err := readFrameLine(r.lines, limit)
		if err != nil {
			return Event{}, false, err
		}
		if !r.isStarted {
			r.isStarted = true
			line = bytes.TrimPrefix(line, []byte(byteOrderMark))
		}
		if line == nil {
			if data != nil {
				return Event{}, false, io.ErrUnexpectedEOF
			}
			return Event{}, false, nil
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
			if size += len(value); limit > 0 && size+len(data)-1 > limit {
				return Event{}, false, frameSizeError(limit)
			}
		case "event":
			e.Type = string(value)
		case "id":
			if bytes.IndexByte(value, 0) < 0 {
				r.lastID = string(value)
			}
		case "retry":
			if retry, ok := parseRetry(value); ok {
				e.Retry = retry
			}
		}
	}
	e.ID, e.Data = r.lastID, bytes.Join(data, []byte("\n"))
	return e, true, nil
}

// lfReader turns every line ending of an event stream, CRLF or a lone CR, into LF, so that a line
// ending in CR is read without waiting for the byte after it.
type lfReader struct {
	reader    io.Reader
	isAfterCR bool
}

func (l *lfReader) Read(p []byte) (int, error) {
	for {
		n, err := l.reader.Read(p)
		kept := 0
		for _, c := range p[:n] {
			switch {
			case c == '\n' && l.isAfterCR:
			case c == '\r':
				p[kept] = '\n'
				kept++
			default:
				p[kept] = c
				kept++
			}
			l.isAfterCR = c == '\r'
		}

		if kept > 0 || n == 0 || err != nil {
			return kept, err
		}
	}
}

// IsSequential reports a media type whose body is a sequence of frames: text/event-stream and
// the line-delimited JSON types, with or without parameters.
func IsSequential(mediaType string) bool {
	mediaType = baseMediaType(mediaType)
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
// A 2xx response without a body, such as 204, is a stream without frames; one in another media
// type is ErrContentType. A status outside 2xx is an *APIError, as DecodeSuccess reports it with
// targets.
func OpenStream[T any](res *http.Response, body []byte, targets []Target) (*Stream[T], error) {
	isSuccess := res.StatusCode >= 200 && res.StatusCode <= 299
	switch {
	case IsStreaming(res), isSuccess && len(body) == 0:
		return NewStream[T](res), nil
	case isSuccess:
		return nil, ContentTypeError(ContentType(res.Header))
	}
	return nil, DecodeSuccess(res, body, targets)
}

// DecodeStream is Decode for a response SendStream returned with its body. A streamed response
// fills the typed headers of its status and comes back as a stream, closed when a header does not
// decode. Any other response is decoded whole and gives no stream.
func DecodeStream[T any](res *http.Response, body []byte, targets []Target) (*Stream[T], error) {
	err := Decode(res, body, targets)
	switch {
	case !IsStreaming(res):
		return nil, err
	case err != nil:
		_ = res.Body.Close()
		return nil, err
	}
	return NewStream[T](res), nil
}

// bodyOf is the body of res, or an empty one when res has none.
func bodyOf(res *http.Response) io.ReadCloser {
	if res.Body == nil {
		return http.NoBody
	}
	return res.Body
}

// readLine reads the next line that holds more than spaces, or reports the end of the body.
func readLine(r *bufio.Reader, limit int) (Event, bool, error) {
	for {
		line, err := readFrameLine(r, limit)
		switch {
		case err != nil:
			return Event{}, false, err
		case line == nil:
			return Event{}, false, nil
		case len(bytes.TrimSpace(line)) > 0:
			return Event{Data: line}, true, nil
		}
	}
}

// readFrameLine reads one line without its line ending, LF or CRLF. A nil line is the end of the
// body; a line without an ending before the end is returned as it is. A line longer than limit,
// when it is above 0, is ErrFrameSize, found before more than the line ending past it is read.
func readFrameLine(r *bufio.Reader, limit int) ([]byte, error) {
	piece, err := r.ReadSlice('\n')
	line := slices.Clone(piece)
	for errors.Is(err, bufio.ErrBufferFull) {
		if limit > 0 && len(line) > limit+len("\r\n") {
			return nil, frameSizeError(limit)
		}
		piece, err = r.ReadSlice('\n')
		line = append(line, piece...)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	if len(line) == 0 {
		return nil, nil
	}
	line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
	if limit > 0 && len(line) > limit {
		return nil, frameSizeError(limit)
	}
	return line, nil
}

func frameSizeError(limit int) error {
	return fmt.Errorf("%w: more than %d bytes", ErrFrameSize, limit)
}

// splitField splits a Server-Sent Events line into its field name and value, dropping the one
// space the value may start with.
func splitField(line []byte) (string, []byte) {
	field, value, _ := bytes.Cut(line, []byte(":"))
	return string(field), bytes.TrimPrefix(value, []byte(" "))
}

// parseRetry reads a retry value: ASCII digits alone, a number of milliseconds a time.Duration
// holds.
func parseRetry(value []byte) (time.Duration, bool) {
	if bytes.ContainsFunc(value, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, false
	}
	ms, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil || ms > int64(math.MaxInt64/time.Millisecond) {
		return 0, false
	}
	return time.Duration(ms) * time.Millisecond, true
}

// decodeFrame reads data into dst: as it is into bytes, and as JSON into anything else.
func decodeFrame(data []byte, dst any) error {
	if raw, ok := dst.(*[]byte); ok {
		*raw = bytes.Clone(data)
		return nil
	}
	return json.Unmarshal(data, dst)
}
