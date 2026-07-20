// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package prepare

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mockzilla/codegen/internal/bundle"
)

const fetchTimeout = 30 * time.Second

// read reads a file, or fetches a URL with a GET that gives up after fetchTimeout.
func read(ctx context.Context, loc string) ([]byte, error) {
	if !bundle.IsURL(loc) {
		data, err := os.ReadFile(loc)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrRead, err)
		}
		return data, nil
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRead, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRead, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s: %s", ErrRead, loc, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrRead, loc, err)
	}
	return data, nil
}
