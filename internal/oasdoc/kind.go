// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

// Kind is the OpenAPI object a node stands for, known from where it sits.
type Kind int

const (
	KindNone Kind = iota
	KindDocument
	KindComponents
	KindPaths
	KindPathItem
	KindOperation
	KindParameter
	KindRequestBody
	KindMediaType
	KindEncoding
	KindResponses
	KindResponse
	KindHeader
	KindSchema
	KindExample
	KindLink
	KindCallback
	KindSecurityScheme
)

// Section is the components key that holds objects of kind k, or "" when there is none.
func (k Kind) Section() string {
	switch k {
	case KindSchema:
		return "schemas"
	case KindResponse:
		return "responses"
	case KindParameter:
		return "parameters"
	case KindExample:
		return "examples"
	case KindRequestBody:
		return "requestBodies"
	case KindHeader:
		return "headers"
	case KindSecurityScheme:
		return "securitySchemes"
	case KindLink:
		return "links"
	case KindCallback:
		return "callbacks"
	case KindPathItem:
		return "pathItems"
	case KindMediaType:
		return "mediaTypes"
	default:
		return ""
	}
}

// SectionKind is the kind of the objects under components/section, or KindNone.
func SectionKind(section string) Kind {
	switch section {
	case "schemas":
		return KindSchema
	case "responses":
		return KindResponse
	case "parameters":
		return KindParameter
	case "examples":
		return KindExample
	case "requestBodies":
		return KindRequestBody
	case "headers":
		return KindHeader
	case "securitySchemes":
		return KindSecurityScheme
	case "links":
		return KindLink
	case "callbacks":
		return KindCallback
	case "pathItems":
		return KindPathItem
	case "mediaTypes":
		return KindMediaType
	default:
		return KindNone
	}
}
