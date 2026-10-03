// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The jobs that passed, so the next run skips them while the tool and the spec stay the same.

package itest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
)

// Cache holds the keys of the jobs that passed with one build of the tool, Tool being its hash.
// A tool with another hash starts with an empty cache, so there is no expiry.
type Cache struct {
	Tool   string          `json:"tool"`
	Passed map[string]bool `json:"passed"`
}

// LoadCache reads the cache file at path for tool. A missing file, or one written for another
// tool, gives an empty cache.
func LoadCache(path, tool string) (*Cache, error) {
	c := &Cache{Tool: tool, Passed: map[string]bool{}}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return c, nil
	case err != nil:
		return nil, err
	}

	var stored Cache
	if err = json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("%w %s: %w", ErrCache, path, err)
	}
	if stored.Tool == tool {
		maps.Copy(c.Passed, stored.Passed)
	}
	return c, nil
}

// Save writes the cache to path.
func (c *Cache) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return errors.Join(enc.Encode(c), f.Close())
}

// Key is the cache key of job: the hash of its spec file and its variant name.
func Key(job Job) (string, error) {
	sum, err := HashFiles(job.Spec.Path)
	if err != nil {
		return "", err
	}
	return sum + " " + job.Variant.Name, nil
}

// HashFiles returns the hex SHA-256 of the files' contents, in order.
func HashFiles(paths ...string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
