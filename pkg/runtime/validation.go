// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"errors"
	"strings"
)

// ValidationError is one failed check. Field is a path like items[2].name, empty for the value itself.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

// ValidationErrors collects failed checks. errors.As finds it, and each ValidationError in it.
type ValidationErrors []ValidationError

func (es ValidationErrors) Error() string {
	msgs := make([]string, len(es))
	for i, e := range es {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

func (es ValidationErrors) Unwrap() []error {
	out := make([]error, len(es))
	for i, e := range es {
		out[i] = e
	}
	return out
}

// Err returns nil when nothing failed, so a Validate method can end with return errs.Err().
func (es ValidationErrors) Err() error {
	if len(es) == 0 {
		return nil
	}
	return es
}

func (es *ValidationErrors) Add(field, message string) {
	*es = append(*es, ValidationError{Field: field, Message: message})
}

// Append adds err under prefix. Nested validation errors are flattened with their paths joined.
func (es *ValidationErrors) Append(prefix string, err error) {
	var many ValidationErrors
	var one ValidationError
	switch {
	case err == nil:
	case errors.As(err, &many):
		for _, e := range many {
			es.Add(joinPath(prefix, e.Field), e.Message)
		}
	case errors.As(err, &one):
		es.Add(joinPath(prefix, one.Field), one.Message)
	default:
		es.Add(prefix, err.Error())
	}
}

func joinPath(prefix, field string) string {
	switch {
	case prefix == "":
		return field
	case field == "":
		return prefix
	case strings.HasPrefix(field, "["):
		return prefix + field
	}
	return prefix + "." + field
}
