// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package streaming

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// chat answers whole as JSON, or as chunks ended by [DONE] when the prompt asks for a stream. An
// empty prompt is rejected.
func chat(w http.ResponseWriter, r *http.Request) {
	var prompt Prompt
	_ = json.NewDecoder(r.Body).Decode(&prompt)
	if prompt.Text == "" {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"detail":"a prompt needs text"}`)
		return
	}
	if prompt.Stream == nil || !*prompt.Stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"text":"hello there"}`)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, "data: {\"text\":\"hello\"}\n\ndata: {\"text\":\" there\",\"done\":true}\n\ndata: [DONE]\n\n")
}

// events sends two events with ids, a comment, a retry hint and data split over two lines, then
// keeps the connection open until the client goes away.
func events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, ": welcome\nretry: 3000\nid: 1\nevent: change\ndata: {\"seq\":1,\"kind\":\"created\",\"at\":\"2026-01-02T03:04:05Z\",\n"+
		"data: \"actor\":{\"name\":\"ada\"},\"tags\":[\"new\"]}\n\nid: 2\ndata: {\"seq\":2,\"kind\":\"deleted\"}\n\n")
	w.(http.Flusher).Flush()
	<-r.Context().Done()
}

// tailLog sends the log lines of job "build" and knows no other job.
func tailLog(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/build") {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"detail":"no such job"}`)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = io.WriteString(w, "{\"line\":\"compiling\"}\r\n\n{\"line\":\"linking\"}\n")
}

func newClient(t *testing.T) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat", chat)
	mux.HandleFunc("GET /events", events)
	mux.HandleFunc("GET /logs/{job}", tailLog)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestChatStream(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	stream, err := c.ChatStream(ctx, &ChatRequestOptions{Body: &Prompt{Text: "hi", Stream: new(true)}})
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()
	stream.Sentinels = []string{"[DONE]"}

	var chunks []Chunk
	for stream.Next() {
		chunks = append(chunks, stream.Current())
	}

	require.NoError(t, stream.Err())
	assert.Equal(t, []Chunk{{Text: "hello"}, {Text: " there", Done: new(true)}}, chunks)
}

func TestChatStreamFallsBackToTheUsualErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	_, err := c.ChatStream(ctx, &ChatRequestOptions{Body: &Prompt{Text: "hi"}})
	require.ErrorIs(t, err, runtime.ErrContentType, "a JSON answer is no stream")

	_, err = c.ChatStream(ctx, &ChatRequestOptions{Body: &Prompt{Text: ""}})
	var problem *Problem
	require.ErrorAs(t, err, &problem)
	assert.Equal(t, "a prompt needs text", problem.Detail)

	reply, err := c.Chat(ctx, &ChatRequestOptions{Body: &Prompt{Text: "hi"}})
	require.NoError(t, err)
	assert.Equal(t, &Reply{Text: "hello there"}, reply, "the plain method is unchanged")
}

func TestChatStreamWithResponse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	res, err := c.ChatStreamWithResponse(ctx, &ChatRequestOptions{Body: &Prompt{Text: "hi", Stream: new(true)}})
	require.NoError(t, err)
	stream := res.Stream200
	require.NotNil(t, stream)
	defer func() { _ = stream.Close() }()

	assert.Equal(t, http.StatusOK, res.StatusCode())
	assert.Nil(t, res.Body, "the body is left to the stream")
	require.True(t, stream.Next())
	assert.Equal(t, Chunk{Text: "hello"}, stream.Current())

	res, err = c.ChatStreamWithResponse(ctx, &ChatRequestOptions{Body: &Prompt{Text: ""}})
	require.NoError(t, err)
	assert.Nil(t, res.Stream200)
	assert.Equal(t, &Problem{Detail: "a prompt needs text"}, res.ProblemJSON400, "any other response is decoded as usual")
}

func TestListEventsStream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := newClient(t)

	stream, err := c.ListEventsStream(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	var items []ListEventsResponseItem
	var events []runtime.Event
	for item, iterErr := range stream.All() {
		require.NoError(t, iterErr)
		items = append(items, item)
		events = append(events, stream.Event())
		if len(items) == 2 {
			break
		}
	}

	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	assert.Equal(t, []ListEventsResponseItem{
		{Seq: 1, Kind: "created", At: &at, Actor: &ListEventsResponseItemActor{Name: "ada"}, Tags: []string{"new"}},
		{Seq: 2, Kind: "deleted"},
	}, items)
	assert.Equal(t, []runtime.Event{
		{ID: "1", Type: "change", Retry: 3 * time.Second, Data: []byte("{\"seq\":1,\"kind\":\"created\",\"at\":\"2026-01-02T03:04:05Z\",\n\"actor\":{\"name\":\"ada\"},\"tags\":[\"new\"]}")},
		{ID: "2", Data: []byte(`{"seq":2,"kind":"deleted"}`)},
	}, events)

	cancel()

	assert.False(t, stream.Next(), "canceling the context ends the wait for the next event")
	require.ErrorIs(t, stream.Err(), context.Canceled)
}

func TestTailLogStream(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	stream, err := c.TailLogStream(ctx, &TailLogRequestOptions{PathParams: &TailLogPathParams{Job: "build"}})
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	var lines []string
	for line, iterErr := range stream.All() {
		require.NoError(t, iterErr)
		lines = append(lines, string(line))
	}

	assert.Equal(t, []string{`{"line":"compiling"}`, `{"line":"linking"}`}, lines)

	_, err = c.TailLogStream(ctx, &TailLogRequestOptions{PathParams: &TailLogPathParams{Job: "deploy"}})
	var apiErr *runtime.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
}

func TestTimeoutBoundsOnlyTheWaitForAStream(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"line\":\"compiling\"}\n")
		w.(http.Flusher).Flush()
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, "{\"line\":\"linking\"}\n")
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, WithTimeout(20*time.Millisecond))
	require.NoError(t, err)
	ctx := context.Background()
	opts := &TailLogRequestOptions{PathParams: &TailLogPathParams{Job: "build"}}

	stream, err := c.TailLogStream(ctx, opts)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()
	var lines []string
	for line, iterErr := range stream.All() {
		require.NoError(t, iterErr)
		lines = append(lines, string(line))
	}

	assert.Equal(t, []string{`{"line":"compiling"}`, `{"line":"linking"}`}, lines, "the second line comes after the timeout")

	_, err = c.TailLog(ctx, opts)
	require.ErrorIs(t, err, context.DeadlineExceeded, "the plain method reads the body whole, within the timeout")
}

func TestInterfaceListsTheStreamMethods(t *testing.T) {
	t.Parallel()

	var c ClientInterface = newClient(t)

	stream, err := c.TailLogStream(context.Background(), &TailLogRequestOptions{PathParams: &TailLogPathParams{Job: "build"}})

	require.NoError(t, err)
	require.NoError(t, stream.Close())
}
