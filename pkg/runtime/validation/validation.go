// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package validation checks values against the constraints of a spec and collects what fails.
package validation

import (
	"errors"
	"strings"
)

// Rule is the keyword of the spec a value fails, as JSON Schema names it.
type Rule string

// The rules of Error.
const (
	RuleRequired             Rule = "required"
	RuleType                 Rule = "type"
	RuleAdditionalProperties Rule = "additionalProperties"
	RuleMinLength            Rule = "minLength"
	RuleMaxLength            Rule = "maxLength"
	RulePattern              Rule = "pattern"
	RuleFormat               Rule = "format"
	RuleMinimum              Rule = "minimum"
	RuleExclusiveMinimum     Rule = "exclusiveMinimum"
	RuleMaximum              Rule = "maximum"
	RuleExclusiveMaximum     Rule = "exclusiveMaximum"
	RuleMultipleOf           Rule = "multipleOf"
	RuleMinItems             Rule = "minItems"
	RuleMaxItems             Rule = "maxItems"
	RuleUniqueItems          Rule = "uniqueItems"
	RuleMinProperties        Rule = "minProperties"
	RuleMaxProperties        Rule = "maxProperties"
	RuleConst                Rule = "const"
	RuleEnum                 Rule = "enum"
	RuleOneOf                Rule = "oneOf"
	RuleAnyOf                Rule = "anyOf"
	RuleDiscriminator        Rule = "discriminator"
)

// Error is one failed check. Field is a path like items[2].name, empty for the value
// itself. Limit is the value of Rule in the spec, nil when the rule has none.
type Error struct {
	Field   string
	Message string
	Rule    Rule
	Limit   any
}

func (e Error) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

// Errors collects failed checks. errors.As finds it as a *Errors, and each Error in it.
type Errors []Error

func (es *Errors) Error() string {
	msgs := make([]string, len(*es))
	for i, e := range *es {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

func (es *Errors) Unwrap() []error {
	out := make([]error, len(*es))
	for i, e := range *es {
		out[i] = e
	}
	return out
}

// Err returns nil when nothing failed, so a Validate method can end with return errs.Err().
func (es *Errors) Err() error {
	if len(*es) == 0 {
		return nil
	}
	// A copy, so the variable the errors were collected in stays off the heap.
	out := *es
	return &out
}

// Add adds a failed check that names no rule.
func (es *Errors) Add(field, message string) {
	*es = append(*es, Error{Field: field, Message: message})
}

// Required adds a required field that is not set.
func (es *Errors) Required(field string) {
	*es = append(*es, Error{Field: field, Message: "is required", Rule: RuleRequired})
}

// Append adds err under prefix. Nested validation errors are flattened with their paths joined.
func (es *Errors) Append(prefix string, err error) {
	var many *Errors
	var one Error
	switch {
	case err == nil:
	case errors.As(err, &many):
		for _, e := range *many {
			e.Field = Join(prefix, e.Field)
			*es = append(*es, e)
		}
	case errors.As(err, &one):
		one.Field = Join(prefix, one.Field)
		*es = append(*es, one)
	default:
		es.Add(prefix, err.Error())
	}
}

// Failed reports whether err holds Errors.
func Failed(err error) bool {
	var errs *Errors
	return errors.As(err, &errs)
}

// Join is the path of field under prefix: pet.name, or pet[0] for an index.
func Join(prefix, field string) string {
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
