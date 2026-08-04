// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
)

// Rank orders requests for the same name: the higher rank keeps it.
type Rank int

const (
	RankInline Rank = iota
	RankOperation
	RankComponent
	RankComponentSchema
	RankGoName
)

// Request asks for one name. ID is unique within a Resolve call, usually a JSON pointer.
type Request struct {
	ID   string
	Want string
	// Fallback is tried before numbering: Fallback2, Fallback3, or Want2 without a Fallback.
	Fallback string
	Rank     Rank
	Order    int
	Origin   diag.Origin
}

type Assignment struct {
	ID   string
	Name string
}

// Rename records a lost name. Holder is the ID that has From, empty when From is reserved.
type Rename struct {
	ID     string
	From   string
	To     string
	Holder string
	Rank   Rank
	Origin diag.Origin
}

// Diagnostic reports the rename; a lost x-go-name is a warning, anything else is info.
func (r Rename) Diagnostic() diag.Diagnostic {
	sev := diag.Info
	if r.Rank == RankGoName {
		sev = diag.Warning
	}

	msg := fmt.Sprintf("%q is reserved, renamed to %q", r.From, r.To)
	if r.Holder != "" {
		msg = fmt.Sprintf("%q is used by %s, renamed to %q", r.From, r.Holder, r.To)
	}
	return diag.Diagnostic{Severity: sev, Code: diag.CodeNameClash, Pointer: r.ID, Origin: r.Origin, Message: msg}
}

// Result maps IDs to names. Range over Ordered, never over Names, to keep output deterministic.
type Result struct {
	Names   map[string]string
	Ordered []Assignment
	Renames []Rename
}

// Resolve compares names ignoring case and goes by rank, spec order and ID, never input order.
func Resolve(reserved []string, reqs []Request) Result {
	holders := make(map[string]string, len(reserved)+len(reqs))
	for _, name := range reserved {
		holders[strings.ToLower(name)] = ""
	}

	sorted := slices.Clone(reqs)
	slices.SortFunc(sorted, func(a, b Request) int {
		return cmp.Or(cmp.Compare(b.Rank, a.Rank), cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
	})

	res := Result{Names: make(map[string]string, len(reqs)), Ordered: make([]Assignment, 0, len(reqs))}
	next := map[string]int{}
	for _, r := range sorted {
		name := pick(holders, next, r)
		if name != r.Want {
			res.Renames = append(res.Renames, Rename{
				ID:     r.ID,
				From:   r.Want,
				To:     name,
				Holder: holders[strings.ToLower(r.Want)],
				Rank:   r.Rank,
				Origin: r.Origin,
			})
		}

		holders[strings.ToLower(name)] = r.ID
		res.Names[r.ID] = name
		res.Ordered = append(res.Ordered, Assignment{ID: r.ID, Name: name})
	}
	return res
}

// next holds the next number per base, so many clashes on one base stay linear.
func pick(holders map[string]string, next map[string]int, r Request) string {
	if _, taken := holders[strings.ToLower(r.Want)]; !taken {
		return r.Want
	}

	base := r.Want
	if r.Fallback != "" {
		if _, taken := holders[strings.ToLower(r.Fallback)]; !taken {
			return r.Fallback
		}
		base = r.Fallback
	}

	key := strings.ToLower(base)
	for i := max(next[key], 2); ; i++ {
		name := base + strconv.Itoa(i)
		if _, taken := holders[strings.ToLower(name)]; !taken {
			next[key] = i + 1
			return name
		}
	}
}
