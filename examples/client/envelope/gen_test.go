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
)

// service runs small jobs at once, queues large ones, and rejects a size of 0.
type service struct{}

func (service) SubmitJob(_ context.Context, opts *SubmitJobServiceRequestOptions) (*SubmitJobResponseData, error) {
	switch size := opts.Body.Size; {
	case size == 0:
		return nil, NewProblem("a job needs a size")
	case size < 10:
		return NewSubmitJobResponseData201(&Result{ID: "j1", Output: "done"}).
			WithTypedHeaders201(SubmitJobResponse201Headers{Location: new("/jobs/j1")}), nil
	}
	return NewSubmitJobResponseData202(&Queued{ID: "j2"}).
		WithTypedHeaders202(SubmitJobResponse202Headers{RetryAfter: new(30), XQueuePosition: new(4)}), nil
}

func (service) GetJobLog(_ context.Context, opts *GetJobLogServiceRequestOptions) (*GetJobLogResponseData, error) {
	switch opts.PathParams.ID {
	case "j1":
		if opts.Headers.Accept != nil && *opts.Headers.Accept == "text/plain" {
			return &GetJobLogResponseData{Status: http.StatusOK, Headers: http.Header{"Content-Type": {"text/plain"}}, Body: "started\ndone"}, nil
		}
		return NewGetJobLogResponseData200(GetJobLogJSONResponse200{"started", "done"}), nil
	}
	return NewGetJobLogResponseData404(), nil
}

func newClient(t *testing.T) *Client {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
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
			want: &SubmitJobResponse{Body: []byte(`{"id":"j2"}`), JSON202: &Queued{ID: "j2"}, Headers202: &SubmitJobResponse202Headers{RetryAfter: new(30), XQueuePosition: new(4)}},
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
	var apiErr *runtime.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.Status)
}

func TestInterfaceListsBothStyles(t *testing.T) {
	t.Parallel()

	var c ClientInterface = newClient(t)

	res, err := c.SubmitJobWithResponse(context.Background(), &SubmitJobRequestOptions{Body: &Job{Size: 1}})

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, res.StatusCode())
}
