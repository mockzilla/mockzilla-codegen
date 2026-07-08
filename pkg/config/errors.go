// Copyright 2026 Mockzilla
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
)

var (
	ErrRead       = errors.New("read config")
	ErrDecode     = errors.New("decode config")
	ErrUnknownKey = errors.New("unknown config key")
	ErrInvalid    = errors.New("invalid config")
	ErrByteSize   = errors.New("invalid byte size")
	ErrDuration   = errors.New("invalid duration")
)

// Issue is one problem in a config, at a dotted key path such as server.framework.
type Issue struct {
	Key     string
	Message string
}

// UnknownKeyError lists every key in a config file that matches no field, as dotted paths.
type UnknownKeyError struct {
	Keys []string
}

func (e *UnknownKeyError) Error() string {
	return ErrUnknownKey.Error() + ": " + strings.Join(e.Keys, ", ")
}

func (e *UnknownKeyError) Unwrap() error {
	return ErrUnknownKey
}

// ValidationError lists every problem Validate found.
type ValidationError struct {
	Issues []Issue
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Issues))
	for i, is := range e.Issues {
		parts[i] = is.Key + ": " + is.Message
	}
	return ErrInvalid.Error() + ": " + strings.Join(parts, "; ")
}

func (e *ValidationError) Unwrap() error {
	return ErrInvalid
}
