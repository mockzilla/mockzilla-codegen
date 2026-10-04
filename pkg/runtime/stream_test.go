// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chunk is a frame of the streams under test.
type chunk struct {
	Text string `json:"text"`
}

// failingBody fails every read, and records Close.
type failingBody struct {
	isClosed bool
}

func (*failingBody) Read([]byte) (int, error) {
	return 0, errRead
}

func (b *failingBody) Close() error {
	b.isClosed = true
	return errRead
}

func streamResponse(status int, contentType, body string) *http.Response {
	res := &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	if contentType != "" {
		res.Header.Set("Content-Type", contentType)
	}
	return res
}

// collect drains s and returns its frames with the events behind them.
func collect[T any](t *testing.T, s *Stream[T]) ([]T, []Event, error) {
	t.Helper()

	var frames []T
	var events []Event
	for s.Next() {
		frames = append(frames, s.Current())
		events = append(events, s.Event())
	}
	return frames, events, s.Err()
}

func TestEventStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantFrames []chunk
		wantEvents []Event
	}{
		{
			name:       "Data lines are joined with newlines",
			body:       "data: {\"text\":\ndata: \"a\"}\n\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{Data: []byte("{\"text\":\n\"a\"}")}},
		},
		{
			name:       "Comments and unknown fields are skipped",
			body:       ": keep-alive\nfoo: bar\ndata: {\"text\":\"a\"}\n\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{Data: []byte(`{"text":"a"}`)}},
		},
		{
			name:       "The id stays for the events that follow, event and retry come with their frame",
			body:       "id: 7\nevent: message\nretry: 1500\ndata: {\"text\":\"a\"}\n\ndata: {\"text\":\"b\"}\n\n",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
			wantEvents: []Event{{ID: "7", Type: "message", Retry: 1500 * time.Millisecond, Data: []byte(`{"text":"a"}`)}, {ID: "7", Data: []byte(`{"text":"b"}`)}},
		},
		{
			name:       "An id in an event without data sets the id, an empty id clears it",
			body:       "id: 1\ndata: {\"text\":\"a\"}\n\nid: 2\n\ndata: {\"text\":\"b\"}\n\nid\ndata: {\"text\":\"c\"}\n\n",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}, {Text: "c"}},
			wantEvents: []Event{{ID: "1", Data: []byte(`{"text":"a"}`)}, {ID: "2", Data: []byte(`{"text":"b"}`)}, {Data: []byte(`{"text":"c"}`)}},
		},
		{
			name:       "An id with a NUL is ignored",
			body:       "id: 1\ndata: {\"text\":\"a\"}\n\nid: 2\x00\ndata: {\"text\":\"b\"}\n\n",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
			wantEvents: []Event{{ID: "1", Data: []byte(`{"text":"a"}`)}, {ID: "1", Data: []byte(`{"text":"b"}`)}},
		},
		{
			name:       "A retry that is not a whole number of milliseconds is ignored",
			body:       "retry: soon\ndata: {\"text\":\"a\"}\n\nretry: -1\ndata: {\"text\":\"b\"}\n\n",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
			wantEvents: []Event{{Data: []byte(`{"text":"a"}`)}, {Data: []byte(`{"text":"b"}`)}},
		},
		{
			name:       "CRLF line endings and a value without a space after the colon",
			body:       "event:tick\r\ndata:{\"text\":\"a\"}\r\n\r\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{Type: "tick", Data: []byte(`{"text":"a"}`)}},
		},
		{
			name:       "A lone CR ends a line, next to LF and CRLF",
			body:       "event: status\rdata: {\"text\":\"a\"}\r\rdata: {\"text\":\r\ndata: \"b\"}\n\r\n",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
			wantEvents: []Event{{Type: "status", Data: []byte(`{"text":"a"}`)}, {Data: []byte("{\"text\":\n\"b\"}")}},
		},
		{
			name:       "A byte order mark at the start is dropped",
			body:       byteOrderMark + "data: {\"text\":\"a\"}\n\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{Data: []byte(`{"text":"a"}`)}},
		},
		{
			name:       "Only one byte order mark is dropped",
			body:       byteOrderMark + byteOrderMark + "data: {\"text\":\"a\"}\n\ndata: {\"text\":\"b\"}\n\n",
			wantFrames: []chunk{{Text: "b"}},
			wantEvents: []Event{{Data: []byte(`{"text":"b"}`)}},
		},
		{
			name:       "A byte order mark after the start is part of its line",
			body:       "data: {\"text\":\"a\"}\n\n" + byteOrderMark + "data: {\"text\":\"b\"}\n\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{Data: []byte(`{"text":"a"}`)}},
		},
		{
			name:       "An event without data is not dispatched and its type is dropped",
			body:       "event: ping\n\nid: 1\ndata: {\"text\":\"a\"}\n\n",
			wantFrames: []chunk{{Text: "a"}},
			wantEvents: []Event{{ID: "1", Data: []byte(`{"text":"a"}`)}},
		},
		{
			name:       "The last event needs no blank line before the end",
			body:       "data: {\"text\":\"a\"}\n\ndata: {\"text\":\"b\"}",
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
			wantEvents: []Event{{Data: []byte(`{"text":"a"}`)}, {Data: []byte(`{"text":"b"}`)}},
		},
		{name: "An empty body"},
		{name: "Comments alone", body: ": hi\n\n: there\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			whole := NewEventStream[chunk](streamResponse(http.StatusOK, MediaTypeEventStream, tc.body))
			bytewise := NewEventStream[chunk](&http.Response{Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(tc.body)))})

			for _, s := range []*Stream[chunk]{whole, bytewise} {
				frames, events, err := collect(t, s)

				require.NoError(t, err)
				assert.Equal(t, tc.wantFrames, frames)
				assert.Equal(t, tc.wantEvents, events)
				assert.False(t, s.Next(), "a finished stream stays finished")
			}
		})
	}
}

func TestEventStreamEndsALineAtCR(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	go func() { _, _ = io.WriteString(pw, "data: a\r\r") }()
	timer := time.AfterFunc(time.Second, func() { _ = pw.CloseWithError(errRead) })
	t.Cleanup(func() { timer.Stop() })
	s := NewEventStream[[]byte](&http.Response{Body: pr})
	t.Cleanup(func() { _ = s.Close() })

	require.True(t, s.Next(), "the event comes without the byte after its CR")
	assert.Equal(t, []byte("a"), s.Current())
}

func TestLFReaderReadsOnPastADroppedLF(t *testing.T) {
	t.Parallel()

	l := &lfReader{reader: iotest.OneByteReader(strings.NewReader("\r\n"))}
	p := make([]byte, 8)

	n, err := l.Read(p)
	require.NoError(t, err)
	assert.Equal(t, "\n", string(p[:n]))
	n, err = l.Read(p)
	assert.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
}

func TestLineStream(t *testing.T) {
	t.Parallel()

	s := NewLineStream[chunk](streamResponse(http.StatusOK, "application/x-ndjson", "{\"text\":\"a\"}\r\n\n{\"text\":\"b\"}\n\n"))

	frames, events, err := collect(t, s)

	require.NoError(t, err)
	assert.Equal(t, []chunk{{Text: "a"}, {Text: "b"}}, frames)
	assert.Equal(t, []Event{{Data: []byte(`{"text":"a"}`)}, {Data: []byte(`{"text":"b"}`)}}, events)
}

func TestNewStreamPicksTheFramingByContentType(t *testing.T) {
	t.Parallel()

	events := NewStream[chunk](streamResponse(http.StatusOK, "text/event-stream; charset=utf-8", "data: {\"text\":\"a\"}\n\n"))
	lines := NewStream[chunk](streamResponse(http.StatusOK, "application/jsonl", "{\"text\":\"a\"}\n"))

	eventFrames, _, err := collect(t, events)
	require.NoError(t, err)
	lineFrames, _, err := collect(t, lines)
	require.NoError(t, err)

	assert.Equal(t, []chunk{{Text: "a"}}, eventFrames)
	assert.Equal(t, []chunk{{Text: "a"}}, lineFrames)
}

func TestStreamSentinels(t *testing.T) {
	t.Parallel()

	s := NewEventStream[chunk](streamResponse(http.StatusOK, MediaTypeEventStream, "data: {\"text\":\"a\"}\n\ndata: [DONE] \n\ndata: {\"text\":\"b\"}\n\n"))
	s.Sentinels = []string{"[DONE]"}

	frames, _, err := collect(t, s)

	require.NoError(t, err)
	assert.Equal(t, []chunk{{Text: "a"}}, frames)
}

func TestStreamOfBytes(t *testing.T) {
	t.Parallel()

	s := NewLineStream[[]byte](streamResponse(http.StatusOK, "application/x-ndjson", "not json\nstill not\n"))

	frames, _, err := collect(t, s)

	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("not json"), []byte("still not")}, frames)
}

func TestStreamErrors(t *testing.T) {
	t.Parallel()

	t.Run("A frame that does not decode", func(t *testing.T) {
		t.Parallel()

		s := NewLineStream[chunk](streamResponse(http.StatusOK, "application/x-ndjson", "{\"text\":\"a\"}\nnope\n"))

		frames, _, err := collect(t, s)

		require.ErrorIs(t, err, ErrFrame)
		assert.Equal(t, []chunk{{Text: "a"}}, frames)
	})
	t.Run("A body that fails to read, and Close that fails", func(t *testing.T) {
		t.Parallel()

		body := &failingBody{}
		events := NewEventStream[chunk](&http.Response{StatusCode: http.StatusOK, Body: body})
		lines := NewLineStream[chunk](&http.Response{StatusCode: http.StatusOK, Body: body})

		for _, s := range []*Stream[chunk]{events, lines} {
			assert.False(t, s.Next())
			require.ErrorIs(t, s.Err(), errRead)
		}
		require.ErrorIs(t, events.Close(), errRead)
		assert.True(t, body.isClosed)
	})
	t.Run("A response without a body is an empty stream", func(t *testing.T) {
		t.Parallel()

		res := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}

		for _, s := range []*Stream[chunk]{NewEventStream[chunk](res), NewLineStream[chunk](res)} {
			assert.False(t, s.Next())
			require.NoError(t, s.Err())
			require.NoError(t, s.Close())
		}
	})
	t.Run("Next after Close reads nothing", func(t *testing.T) {
		t.Parallel()

		s := NewLineStream[chunk](streamResponse(http.StatusOK, "application/x-ndjson", "{\"text\":\"a\"}\n"))

		require.NoError(t, s.Close())

		assert.False(t, s.Next())
		require.NoError(t, s.Err())
	})
}

func TestStreamAll(t *testing.T) {
	t.Parallel()

	t.Run("Frames, then the error", func(t *testing.T) {
		t.Parallel()

		s := NewLineStream[chunk](streamResponse(http.StatusOK, "application/x-ndjson", "{\"text\":\"a\"}\nnope\n"))

		var frames []chunk
		var errs []error
		for c, err := range s.All() {
			frames, errs = append(frames, c), append(errs, err)
		}

		assert.Equal(t, []chunk{{Text: "a"}, {}}, frames)
		require.Len(t, errs, 2)
		require.NoError(t, errs[0])
		require.ErrorIs(t, errs[1], ErrFrame)
	})
	t.Run("Stopping early", func(t *testing.T) {
		t.Parallel()

		s := NewLineStream[chunk](streamResponse(http.StatusOK, "application/x-ndjson", "{\"text\":\"a\"}\n{\"text\":\"b\"}\n"))

		var frames []chunk
		for c := range s.All() {
			frames = append(frames, c)
			break
		}

		assert.Equal(t, []chunk{{Text: "a"}}, frames)
		assert.True(t, s.Next(), "the stream goes on after the loop")
	})
}

func TestStreamStopUnblocksNext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stop    func(context.CancelFunc, *Stream[chunk])
		wantErr error
	}{
		{
			name:    "Canceling the request's context",
			stop:    func(cancel context.CancelFunc, _ *Stream[chunk]) { cancel() },
			wantErr: context.Canceled,
		},
		{
			name: "Closing the stream",
			stop: func(_ context.CancelFunc, s *Stream[chunk]) { _ = s.Close() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			done := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", MediaTypeEventStream)
				_, _ = io.WriteString(w, "data: {\"text\":\"a\"}\n\n")
				w.(http.Flusher).Flush()
				<-done
			}))
			t.Cleanup(srv.Close)
			t.Cleanup(func() { close(done) })

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			res, body, err := SendStream(srv.Client(), req, MediaTypeEventStream, 0)
			require.NoError(t, err)
			s, err := OpenStream[chunk](res, body, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = s.Close() })

			require.True(t, s.Next())
			assert.Equal(t, chunk{Text: "a"}, s.Current())
			go func() {
				time.Sleep(50 * time.Millisecond)
				tc.stop(cancel, s)
			}()

			assert.False(t, s.Next())
			require.ErrorIs(t, s.Err(), tc.wantErr)
		})
	}
}

func TestIsSequential(t *testing.T) {
	t.Parallel()

	for _, mt := range []string{"text/event-stream", "application/x-ndjson", "application/ndjson", "application/jsonl", "application/x-jsonlines", "application/json-lines"} {
		assert.True(t, IsSequential(mt), mt)
	}
	assert.True(t, IsSequential("Text/Event-Stream; charset=utf-8"))
	assert.False(t, IsSequential("application/json"))
	assert.False(t, IsSequential("application/stream+json"))
}

func TestIsStreaming(t *testing.T) {
	t.Parallel()

	assert.True(t, IsStreaming(streamResponse(http.StatusOK, "text/event-stream; charset=utf-8", "")))
	assert.False(t, IsStreaming(streamResponse(http.StatusNotFound, MediaTypeEventStream, "")))
	assert.False(t, IsStreaming(streamResponse(http.StatusOK, "application/json", "")))
	assert.False(t, IsStreaming(&http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {MediaTypeEventStream}}}))
}

func TestSendStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		accept     string
		res        *http.Response
		err        error
		wantAccept string
		wantBody   []byte
		wantOpen   bool
	}{
		{name: "A streaming response keeps its body open", res: streamResponse(http.StatusOK, MediaTypeEventStream, "data: x\n\n"), wantAccept: MediaTypeEventStream, wantOpen: true},
		{name: "An Accept the request sets stays", accept: "*/*", res: streamResponse(http.StatusOK, MediaTypeEventStream, "data: x\n\n"), wantAccept: "*/*", wantOpen: true},
		{name: "A JSON response is read whole", res: streamResponse(http.StatusOK, "application/json", `{}`), wantAccept: MediaTypeEventStream, wantBody: []byte(`{}`)},
		{name: "An error response is read whole", res: streamResponse(http.StatusBadGateway, MediaTypeEventStream, "down"), wantAccept: MediaTypeEventStream, wantBody: []byte("down")},
		{name: "A failed request", err: errRead, wantAccept: MediaTypeEventStream},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://api.test/events", nil)
			require.NoError(t, err)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			var sent *http.Request
			d := doerFunc(func(r *http.Request) (*http.Response, error) {
				sent = r
				return tc.res, tc.err
			})

			res, body, err := SendStream(d, req, MediaTypeEventStream, time.Minute)

			assert.Equal(t, tc.wantAccept, req.Header.Get("Accept"))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				require.ErrorIs(t, sent.Context().Err(), context.Canceled)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantBody, body)
			if !tc.wantOpen {
				require.ErrorIs(t, sent.Context().Err(), context.Canceled)
				return
			}
			rest, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Equal(t, "data: x\n\n", string(rest))
			require.NoError(t, sent.Context().Err(), "the request lives while the stream is open")
			require.NoError(t, res.Body.Close())
			require.ErrorIs(t, sent.Context().Err(), context.Canceled)
		})
	}
}

func TestSendStreamTimeout(t *testing.T) {
	t.Parallel()

	const timeout = 20 * time.Millisecond
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantFrames []chunk
		wantErr    error
	}{
		{
			name: "Headers that come too late time out",
			handler: func(_ http.ResponseWriter, r *http.Request) {
				hold(r)
			},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "A stream outlives the timeout",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", MediaTypeEventStream)
				_, _ = io.WriteString(w, "data: {\"text\":\"a\"}\n\n")
				w.(http.Flusher).Flush()
				time.Sleep(3 * timeout)
				_, _ = io.WriteString(w, "data: {\"text\":\"b\"}\n\n")
			},
			wantFrames: []chunk{{Text: "a"}, {Text: "b"}},
		},
		{
			name: "An answer that is no stream is bounded as a whole",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				w.(http.Flusher).Flush()
				hold(r)
			},
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)

			res, _, err := SendStream(srv.Client(), req, MediaTypeEventStream, timeout)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			s := NewStream[chunk](res)
			t.Cleanup(func() { _ = s.Close() })
			frames, _, err := collect(t, s)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFrames, frames)
		})
	}
}

func TestOpenStream(t *testing.T) {
	t.Parallel()

	targets := []Target{{Status: "default", MediaType: "application/json", Dst: &notFound{}}}
	tests := []struct {
		name       string
		res        *http.Response
		wantFrames []chunk
		wantErr    error
		wantStatus int
	}{
		{name: "A stream", res: streamResponse(http.StatusOK, "application/ndjson", "{\"text\":\"a\"}\n"), wantFrames: []chunk{{Text: "a"}}},
		{name: "A 2xx in another media type", res: streamResponse(http.StatusOK, "application/json", `{"text":"a"}`), wantErr: ErrContentType},
		{name: "A 2xx without a body", res: &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}}, wantErr: ErrContentType},
		{name: "An error response decoded into its type", res: streamResponse(http.StatusNotFound, "application/json", `{"message":"gone"}`), wantErr: &notFound{Message: "gone"}, wantStatus: http.StatusNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://api.test/events", nil)
			require.NoError(t, err)
			d := doerFunc(func(*http.Request) (*http.Response, error) { return tc.res, nil })
			res, body, err := SendStream(d, req, "application/ndjson", 0)
			require.NoError(t, err)

			s, err := OpenStream[chunk](res, body, targets)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.Nil(t, s)
				if tc.wantStatus != 0 {
					var apiErr *APIError
					require.ErrorAs(t, err, &apiErr)
					assert.Equal(t, tc.wantStatus, apiErr.Status)
					assert.Equal(t, tc.wantErr, apiErr.Err)
					return
				}
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			frames, _, err := collect(t, s)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFrames, frames)
		})
	}
}

func TestDecodeStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		header      http.Header
		body        string
		wantFrames  []chunk
		wantJSON    *chunk
		wantHeaders *pageHeaders
		wantErr     string
	}{
		{
			name:        "A stream fills the typed headers of its status",
			contentType: "application/ndjson",
			header:      http.Header{"X-Total": {"5"}},
			body:        "{\"text\":\"a\"}\n",
			wantFrames:  []chunk{{Text: "a"}},
			wantHeaders: &pageHeaders{Total: Ptr(5)},
		},
		{
			name:        "A stream whose header does not decode is closed",
			contentType: "application/ndjson",
			header:      http.Header{"X-Total": {"x"}},
			body:        "{\"text\":\"a\"}\n",
			wantErr:     `invalid parameter value: "x" is no int`,
		},
		{
			name:        "Any other response is decoded whole",
			contentType: "application/json",
			header:      http.Header{"X-Total": {"5"}},
			body:        `{"text":"b"}`,
			wantJSON:    &chunk{Text: "b"},
			wantHeaders: &pageHeaders{Total: Ptr(5)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := &closingBody{Reader: strings.NewReader(tc.body)}
			sent := response(http.StatusOK, tc.contentType, tc.header)
			sent.Body = body
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://api.test/events", nil)
			require.NoError(t, err)
			d := doerFunc(func(*http.Request) (*http.Response, error) { return sent, nil })
			res, data, err := SendStream(d, req, "application/ndjson", 0)
			require.NoError(t, err)

			var json200 *chunk
			var headers *pageHeaders
			s, err := DecodeStream[chunk](res, data, []Target{
				{Status: "200", MediaType: "application/json", Dst: &json200},
				{Status: "200", IsHeaders: true, Dst: &headers},
			})

			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				assert.Nil(t, s)
				assert.True(t, body.isClosed)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantJSON, json200)
			assert.Equal(t, tc.wantHeaders, headers)
			if tc.wantFrames == nil {
				assert.Nil(t, s)
				return
			}
			assert.False(t, body.isClosed, "the stream holds the body")
			frames, _, err := collect(t, s)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFrames, frames)
			require.NoError(t, s.Close())
			assert.True(t, body.isClosed)
		})
	}
}

func TestParseRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  string
		want   time.Duration
		wantOK bool
	}{
		{name: "Milliseconds", value: "1500", want: 1500 * time.Millisecond, wantOK: true},
		{name: "The most milliseconds a Duration holds", value: "9223372036854", want: 9223372036854 * time.Millisecond, wantOK: true},
		{name: "No value", value: ""},
		{name: "A word", value: "soon"},
		{name: "A minus sign", value: "-1"},
		{name: "A plus sign", value: "+5"},
		{name: "More milliseconds than a Duration holds", value: "9223372036855"},
		{name: "More than an int64 holds", value: "99999999999999999999"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseRetry([]byte(tc.value))

			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantOK, ok)
		})
	}
}
