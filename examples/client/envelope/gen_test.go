// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package envelope

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
	"github.com/mockzilla/mockzilla-codegen/pkg/runtime/httpclient"
)

// submitJob runs small jobs at once, queues large ones, and rejects a size of 0.
func submitJob(w http.ResponseWriter, r *http.Request) {
	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch {
	case job.Size == 0:
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"detail":"a job needs a size"}`)
	case job.Size < 10:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/jobs/j1")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"j1","output":"done"}`)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.Header().Set("X-Queue-Position", "4")
		w.Header().Set("X-Queue", "position=4,length=9")
		w.Header().Set("X-Estimate", `{"seconds":30}`)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"id":"j2"}`)
	}
}

// getJobLog knows only job j1, and sends its log as text when asked for text/plain.
func getJobLog(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.PathValue("id") != "j1":
		w.WriteHeader(http.StatusNotFound)
	case r.Header.Get("Accept") == "text/plain":
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "started\ndone")
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `["started","done"]`)
	}
}

func newClient(t *testing.T) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", submitJob)
	mux.HandleFunc("GET /jobs/{id}/log", getJobLog)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestSubmitJobWithResponse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name string
		size int
		want *SubmitJobResponse
	}{
		{
			name: "A job done at once",
			size: 1,
			want: &SubmitJobResponse{Body: []byte(`{"id":"j1","output":"done"}`), JSON201: &Result{ID: "j1", Output: "done"}, Headers201: &SubmitJobResponse201Headers{Location: new("/jobs/j1")}},
		},
		{
			name: "A job queued",
			size: 100,
			want: &SubmitJobResponse{Body: []byte(`{"id":"j2"}`), JSON202: &Queued{ID: "j2"}, Headers202: &SubmitJobResponse202Headers{
				RetryAfter:     new(30),
				XQueuePosition: new(4),
				XQueue:         &SubmitJobResponse202HeadersXQueue{Position: new(4), Length: new(9)},
				XEstimate:      &SubmitJobResponse202HeadersXEstimate{Seconds: new(30)},
			}},
		},
		{
			name: "A job rejected is no error",
			size: 0,
			want: &SubmitJobResponse{Body: []byte(`{"detail":"a job needs a size"}`), ProblemJSON400: &Problem{Detail: "a job needs a size"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := c.SubmitJobWithResponse(ctx, &SubmitJobRequestOptions{Body: &Job{Size: tc.size}})

			require.NoError(t, err)
			require.NotNil(t, res.HTTPResponse)
			assert.Equal(t, res.HTTPResponse.StatusCode, res.StatusCode())
			res.HTTPResponse = nil
			assert.Equal(t, tc.want, res)
		})
	}
}

func TestSubmitJobWithResponseKeepsTheResponseOfABodyThatDoesNotDecode(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":1}`)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	res, err := c.SubmitJobWithResponse(context.Background(), &SubmitJobRequestOptions{Body: &Job{Size: 1}})

	var typeErr *json.UnmarshalTypeError
	require.ErrorAs(t, err, &typeErr)
	require.NotNil(t, res)
	assert.Equal(t, http.StatusCreated, res.StatusCode())
	assert.Equal(t, []byte(`{"id":1}`), res.Body)
}

func TestSubmitJobPicksTheLowestSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	result, err := c.SubmitJob(ctx, &SubmitJobRequestOptions{Body: &Job{Size: 1}})
	require.NoError(t, err)
	assert.Equal(t, &Result{ID: "j1", Output: "done"}, result)

	result, err = c.SubmitJob(ctx, &SubmitJobRequestOptions{Body: &Job{Size: 100}})
	require.NoError(t, err)
	assert.Nil(t, result, "a 202 has no field in the plain method")

	_, err = c.SubmitJob(ctx, &SubmitJobRequestOptions{Body: &Job{Size: 0}})
	var problem *Problem
	require.ErrorAs(t, err, &problem)
	assert.Equal(t, "a job needs a size", problem.Detail)
}

func TestGetJobLogWithResponse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name   string
		id     string
		accept *string
		want   *GetJobLogResponse
	}{
		{name: "As JSON", id: "j1", want: &GetJobLogResponse{Body: []byte(`["started","done"]`), JSON200: GetJobLogJSONResponse200{"started", "done"}}},
		{name: "As text", id: "j1", accept: new("text/plain"), want: &GetJobLogResponse{Body: []byte("started\ndone"), Text200: new("started\ndone")}},
		{name: "Not found, with nothing decoded", id: "j9", want: &GetJobLogResponse{Body: []byte{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := c.GetJobLogWithResponse(ctx, &GetJobLogRequestOptions{PathParams: &GetJobLogPathParams{ID: tc.id}, Headers: &GetJobLogHeaders{Accept: tc.accept}})

			require.NoError(t, err)
			res.HTTPResponse = nil
			assert.Equal(t, tc.want, res)
		})
	}
}

func TestGetJobLogPlain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	lines, err := c.GetJobLog(ctx, &GetJobLogRequestOptions{PathParams: &GetJobLogPathParams{ID: "j1"}})
	require.NoError(t, err)
	assert.Equal(t, GetJobLogJSONResponse200{"started", "done"}, lines)

	_, err = c.GetJobLog(ctx, &GetJobLogRequestOptions{PathParams: &GetJobLogPathParams{ID: "j1"}, Headers: &GetJobLogHeaders{Accept: new("text/plain")}})
	require.ErrorIs(t, err, runtime.ErrContentType, "the plain method takes JSON only")

	_, err = c.GetJobLog(ctx, &GetJobLogRequestOptions{PathParams: &GetJobLogPathParams{ID: "j9"}})
	var apiErr *httpclient.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
}

func TestInterfaceListsBothStyles(t *testing.T) {
	t.Parallel()

	var c ClientInterface = newClient(t)

	res, err := c.SubmitJobWithResponse(context.Background(), &SubmitJobRequestOptions{Body: &Job{Size: 1}})

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode())
}
