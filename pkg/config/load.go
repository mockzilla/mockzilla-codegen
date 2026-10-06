// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"go.yaml.in/yaml/v4"
)

// Load reads and parses the config file at path. Relative paths in it resolve against the
// directory of the file.
func Load(path string, edits ...func(*Config)) (*Config, error) {
	// Abs fails only when the working directory is gone, so it shares the read error path.
	abs, err := filepath.Abs(path)
	if err == nil {
		var data []byte
		if data, err = os.ReadFile(abs); err == nil {
			return Parse(data, filepath.Dir(abs), edits...)
		}
	}
	return nil, fmt.Errorf("%w: %w", ErrRead, err)
}

// Parse decodes a config, rejects unknown keys, runs the edits, fills in defaults and validates the
// result. Relative paths in it resolve against dir.
func Parse(data []byte, dir string, edits ...func(*Config)) (*Config, error) {
	cfg := &Config{dir: dir}
	if err := decode(data, cfg); err != nil {
		return nil, err
	}

	for _, edit := range edits {
		edit(cfg)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func decode(data []byte, cfg *Config) error {
	var doc yaml.Node
	err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&doc)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecode, err)
	}

	if keys := walk(doc.Content[0], reflect.TypeFor[Config](), ""); len(keys) > 0 {
		return &UnknownKeyError{Keys: keys}
	}

	if err = doc.Load(cfg, yaml.WithKnownFields()); err != nil {
		return fmt.Errorf("%w: %w", ErrDecode, err)
	}
	return nil
}
