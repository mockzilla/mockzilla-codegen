// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"encoding/json"
	"reflect"
)

// Nullable is a value that may be absent or null. Its zero value is absent.
type Nullable[T any] struct {
	// A pointer, so a type can hold a Nullable of itself.
	value *T
	isSet bool
}

// nullable is how reflection reaches the value of a Nullable of any type.
type nullable interface {
	elem() (reflect.Value, bool)
	elemType() reflect.Type
}

// nullableTarget is a Nullable that reflection sets.
type nullableTarget interface {
	target() reflect.Value
}

var nullableType = reflect.TypeFor[nullable]()

// Some returns a Nullable that holds v.
func Some[T any](v T) Nullable[T] {
	return Nullable[T]{value: &v, isSet: true}
}

// Null returns a Nullable that is null.
func Null[T any]() Nullable[T] {
	return Nullable[T]{isSet: true}
}

// Get returns the value and true, or the zero value and false when it is absent or null.
func (n Nullable[T]) Get() (T, bool) {
	if n.value == nil {
		var zero T
		return zero, false
	}
	return *n.value, true
}

// Or returns the value, or def when it is absent or null.
func (n Nullable[T]) Or(def T) T {
	if n.value == nil {
		return def
	}
	return *n.value
}

// IsNull reports whether it is null.
func (n Nullable[T]) IsNull() bool {
	return n.isSet && n.value == nil
}

// IsSet reports whether it holds a value or null.
func (n Nullable[T]) IsSet() bool {
	return n.isSet
}

// IsZero reports whether it is absent, which omitzero leaves out.
func (n Nullable[T]) IsZero() bool {
	return !n.isSet
}

// MarshalJSON writes the value, or null when it holds none.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(n.value)
}

// UnmarshalJSON reads a value, or null.
func (n *Nullable[T]) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*n = Null[T]()
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*n = Some(v)
	return nil
}

func (n Nullable[T]) elem() (reflect.Value, bool) {
	if n.value == nil {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(n.value).Elem(), true
}

func (Nullable[T]) elemType() reflect.Type {
	return reflect.TypeFor[T]()
}

func (n *Nullable[T]) target() reflect.Value {
	if n.value == nil {
		n.value = new(T)
	}
	n.isSet = true
	return reflect.ValueOf(n.value).Elem()
}

// valueType is the type t holds behind pointers and Nullables.
func valueType(t reflect.Type) reflect.Type {
	for {
		switch {
		case t.Kind() == reflect.Pointer:
			t = t.Elem()
		case t.Implements(nullableType):
			n, _ := reflect.Zero(t).Interface().(nullable)
			t = n.elemType()
		default:
			return t
		}
	}
}

// held is what v holds behind pointers, interfaces and Nullables, and false when that is nothing.
func held(v reflect.Value) (reflect.Value, bool) {
	for {
		switch {
		case !v.IsValid():
			return v, false
		case v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface:
			if v.IsNil() {
				return v, false
			}
			v = v.Elem()
		case v.CanInterface() && v.Type().Implements(nullableType):
			n, _ := v.Interface().(nullable)
			inner, ok := n.elem()
			if !ok {
				return v, false
			}
			v = inner
		default:
			return v, true
		}
	}
}

// targetOf sets the Nullable dst and returns its value to write, or false when dst is none.
func targetOf(dst reflect.Value) (reflect.Value, bool) {
	if !dst.CanAddr() {
		return reflect.Value{}, false
	}
	n, ok := dst.Addr().Interface().(nullableTarget)
	if !ok {
		return reflect.Value{}, false
	}
	return n.target(), true
}
