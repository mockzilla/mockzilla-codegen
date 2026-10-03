// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package spec

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	InPath        = "path"
	InQuery       = "query"
	InHeader      = "header"
	InCookie      = "cookie"
	InQueryString = "querystring"
)

// MethodOrder is the order operations of one path item come in; 3.2 additional operations follow.
var MethodOrder = []string{"GET", "PUT", "POST", "DELETE", "OPTIONS", "HEAD", "PATCH", "TRACE", "QUERY"}

// Operation is one method on a path, webhook or callback expression.
type Operation struct {
	ID          string
	IsIDDerived bool
	Method      string
	Path        string
	IsWebhook   bool
	Summary     string
	Description string
	Deprecated  bool
	Tags        []string
	// Path-level params come first; an operation param with the same in and name replaces one.
	Params    []*Parameter
	Body      *RequestBody
	Responses []*Response
	Callbacks []*Callback
	// Security is the effective list: the operation's own when it sets one, else the document's.
	Security   []SecurityRequirement
	Servers    []Server
	Extensions []Extension
	Origin     Origin
}

// ComponentRef records the $ref an object came from; Name is set for a component target.
type ComponentRef struct {
	Pointer string
	Name    string
}

// Parameter has Style and Explode resolved to their defaults when the spec leaves them out.
type Parameter struct {
	Name            string
	In              string
	Description     string
	Required        bool
	Deprecated      bool
	AllowEmptyValue bool
	Style           string
	Explode         bool
	AllowReserved   bool
	Schema          *Schema
	Contents        []*MediaType
	Extensions      []Extension
	Ref             *ComponentRef
	Origin          Origin
}

type RequestBody struct {
	Description string
	Required    bool
	Contents    []*MediaType
	Extensions  []Extension
	Ref         *ComponentRef
	Origin      Origin
}

type MediaType struct {
	Name       string
	Schema     *Schema
	ItemSchema *Schema
	Encodings  []*Encoding
	Extensions []Extension
	Origin     Origin
}

type Encoding struct {
	Name          string
	ContentType   string
	Headers       []*Header
	Style         string
	Explode       *bool
	AllowReserved bool
}

// Response is one entry of an operation's responses. Status is empty on a component response.
type Response struct {
	Status      string
	Description string
	Headers     []*Header
	Contents    []*MediaType
	Extensions  []Extension
	Ref         *ComponentRef
	Origin      Origin
}

type Header struct {
	Name        string
	Description string
	Required    bool
	Deprecated  bool
	Style       string
	Explode     bool
	Schema      *Schema
	Contents    []*MediaType
	Extensions  []Extension
	Ref         *ComponentRef
	Origin      Origin
}

// Callback holds the operations of all its expressions; each operation's Path is its expression.
type Callback struct {
	Name       string
	Operations []*Operation
	Ref        *ComponentRef
}

// DeriveOperationID names an operation without operationId: GET /users/{id} gives getUsersId.
func DeriveOperationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))

	words := strings.FieldsFunc(path, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		b.WriteRune(unicode.ToUpper(r))
		b.WriteString(w[size:])
	}
	return b.String()
}

// StatusCode reads a status code from a response key: the key when it is a number, the first digit
// times 100 when it has three characters and starts with a digit, as a range such as 4XX does. It
// reads none from default or from another key.
func StatusCode(status string) (int, bool) {
	if code, err := strconv.Atoi(status); err == nil {
		return code, true
	}
	if len(status) == 3 {
		if code, err := strconv.Atoi(status[:1]); err == nil {
			return code * 100, true
		}
	}
	return 0, false
}
