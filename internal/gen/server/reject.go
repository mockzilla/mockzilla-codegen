// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// The error types of the spec that answer the requests the adapter turns away.

package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/gen/operation"
	"github.com/mockzilla/mockzilla-codegen/internal/gocode"
	"github.com/mockzilla/mockzilla-codegen/internal/gomodel"
	"github.com/mockzilla/mockzilla-codegen/internal/layout"
)

// RejectView answers the requests the operations of IDs, each quoted, turn away.
type RejectView struct {
	IDs   []string
	Cases []RejectCaseView
}

// RejectCaseView is the error type that answers one status, every status when Status is 0. New
// is its constructor and MediaType is quoted.
type RejectCaseView struct {
	Status    int
	New       string
	MediaType string
}

// rejection is the error type that answers a request turned away with status, or with any status
// when it is 0.
type rejection struct {
	status    int
	decl      *gomodel.Decl
	mediaType string
}

func rejectViews(ops []*gomodel.Operation, s *gocode.Scope) []RejectView {
	var out []RejectView
	var groups [][]rejection
	for _, op := range ops {
		rs, _ := rejections(op)
		if len(rs) == 0 {
			continue
		}
		i := slices.IndexFunc(groups, func(group []rejection) bool { return slices.Equal(group, rs) })
		if i < 0 {
			groups = append(groups, rs)
			out = append(out, RejectView{Cases: rejectCases(rs, s)})
			i = len(out) - 1
		}
		out[i].IDs = append(out[i].IDs, gocode.Quote(op.Name))
	}
	return out
}

func rejectCases(rs []rejection, s *gocode.Scope) []RejectCaseView {
	out := make([]RejectCaseView, len(rs))
	for i, r := range rs {
		out[i] = RejectCaseView{
			Status:    r.status,
			New:       s.Symbol(layout.PartID(r.decl.Part), "New"+r.decl.Name),
			MediaType: gocode.Quote(r.mediaType),
		}
	}
	return out
}

// rejectWarnings warn, once per type, about an error type that answers the requests the adapter
// turns away and leaves required properties empty, or that has no constructor to build it with.
func rejectWarnings(ops []*gomodel.Operation) []diag.Diagnostic {
	var out []diag.Diagnostic
	var seen []*gomodel.Decl
	for _, op := range ops {
		rs, unbuilt := rejections(op)
		for _, r := range rs {
			if slices.Contains(seen, r.decl) {
				continue
			}
			seen = append(seen, r.decl)
			if e := r.decl.Error; len(e.Unset) > 0 {
				out = append(out, declWarning(r.decl, fmt.Sprintf("%s answers requests the server turns away with %s set and these required properties empty: %s",
					r.decl.Name, e.Path, strings.Join(e.Unset, ", "))))
			}
		}
		for _, d := range unbuilt {
			if slices.Contains(seen, d) {
				continue
			}
			seen = append(seen, d)
			out = append(out, declWarning(d, d.Name+` has no constructor, so requests the server turns away are answered with {"error": ...}`))
		}
	}
	return out
}

// rejections are the error types op answers the requests it turns away with, one of status 0 when
// a single type answers them all, and the mapped types it documents for them that have no
// constructor.
func rejections(op *gomodel.Operation) ([]rejection, []*gomodel.Decl) {
	statuses := []int{http.StatusBadRequest}
	isWildcard := func(c gomodel.Content) bool { return strings.Contains(operation.BaseMediaType(c.MediaType), "*") }
	if len(op.Bodies) > 0 && !slices.ContainsFunc(op.Bodies, isWildcard) {
		statuses = append(statuses, http.StatusUnsupportedMediaType)
	}

	var out []rejection
	var unbuilt []*gomodel.Decl
	for _, status := range statuses {
		r, ok := documented(op, status)
		if !ok {
			continue
		}
		i := slices.IndexFunc(r.Contents, func(c gomodel.Content) bool { return gomodel.ErrorDecl(c.Type) != nil })
		if i < 0 {
			continue
		}
		d := gomodel.ErrorDecl(r.Contents[i].Type)
		if !d.Error.HasConstructor {
			unbuilt = append(unbuilt, d)
			continue
		}
		out = append(out, rejection{status: status, decl: d, mediaType: r.Contents[i].MediaType})
	}

	isOne := len(out) == len(statuses)
	for _, r := range out {
		isOne = isOne && r.decl == out[0].decl && r.mediaType == out[0].mediaType
	}
	if isOne {
		return []rejection{{decl: out[0].decl, mediaType: out[0].mediaType}}, unbuilt
	}
	return out, unbuilt
}

// documented is the response op documents for status: the one of its code, else of its range,
// else default.
func documented(op *gomodel.Operation, status int) (gomodel.Response, bool) {
	for _, key := range []string{strconv.Itoa(status), strconv.Itoa(status/100) + "XX", "default"} {
		i := slices.IndexFunc(op.Responses, func(r gomodel.Response) bool { return strings.EqualFold(r.Status, key) })
		if i >= 0 {
			return op.Responses[i], true
		}
	}
	return gomodel.Response{}, false
}

func declWarning(d *gomodel.Decl, message string) diag.Diagnostic {
	return diag.Diagnostic{Severity: diag.Warning, Code: diag.CodeErrorMapping, Pointer: d.ID, Origin: d.Origin, Message: message}
}
