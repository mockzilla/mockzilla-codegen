// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Clashes: when several places ask for one name, the highest rank keeps it. The others take their
// fallback name, or a number.

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
	// Methods are the suffixes of the methods built from the name: the request holds its name
	// plus each of them too, so a name is free only when all of these are.
	Methods []string
	Rank    Rank
	Order   int
	Origin  diag.Origin
}

type Assignment struct {
	ID   string
	Name string
}

// Rename records a lost name. From is the name that was taken: Want, or Want plus one of its
// Methods. Holder is the ID that has From, empty when From is reserved; IsMethod says Holder has
// it as one of its methods.
type Rename struct {
	ID       string
	From     string
	To       string
	Holder   string
	IsMethod bool
	Rank     Rank
	Origin   diag.Origin
}

// Diagnostic reports the rename; a lost x-go-name is a warning, anything else is info.
func (r Rename) Diagnostic() diag.Diagnostic {
	sev := diag.Info
	if r.Rank == RankGoName {
		sev = diag.Warning
	}

	msg := fmt.Sprintf("%q is reserved, renamed to %q", r.From, r.To)
	switch {
	case r.IsMethod:
		msg = fmt.Sprintf("%q is a method of %s, renamed to %q", r.From, r.Holder, r.To)
	case r.Holder != "":
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

// holder is the request that has a name, with an empty ID for a reserved name.
type holder struct {
	id       string
	isMethod bool
}

// holders maps names in lower case to who has them.
type holders map[string]holder

// taken is the first of name and name plus each suffix that someone has.
func (h holders) taken(name string, suffixes []string) (string, bool) {
	if _, ok := h[strings.ToLower(name)]; ok {
		return name, true
	}
	for _, s := range suffixes {
		if _, ok := h[strings.ToLower(name+s)]; ok {
			return name + s, true
		}
	}
	return "", false
}

// Resolve compares names ignoring case and goes by rank, spec order and ID, never input order. A
// request whose Want is another one's Want plus one of its Methods goes after that one, so it is
// the one renamed.
func Resolve(reserved []string, reqs []Request) Result {
	held := make(holders, len(reserved)+len(reqs))
	for _, name := range reserved {
		held[strings.ToLower(name)] = holder{}
	}

	depth := depths(reqs)
	sorted := slices.Clone(reqs)
	slices.SortFunc(sorted, func(a, b Request) int {
		return cmp.Or(cmp.Compare(b.Rank, a.Rank), cmp.Compare(depth[a.ID], depth[b.ID]), cmp.Compare(a.Order, b.Order), cmp.Compare(a.ID, b.ID))
	})

	res := Result{Names: make(map[string]string, len(reqs)), Ordered: make([]Assignment, 0, len(reqs))}
	next := map[string]int{}
	for _, r := range sorted {
		name := pick(held, next, r)
		if name != r.Want {
			from, _ := held.taken(r.Want, r.Methods)
			h := held[strings.ToLower(from)]
			res.Renames = append(res.Renames, Rename{
				ID:       r.ID,
				From:     from,
				To:       name,
				Holder:   h.id,
				IsMethod: h.isMethod,
				Rank:     r.Rank,
				Origin:   r.Origin,
			})
		}

		held[strings.ToLower(name)] = holder{id: r.ID}
		for _, s := range r.Methods {
			held[strings.ToLower(name+s)] = holder{id: r.ID, isMethod: true}
		}
		res.Names[r.ID] = name
		res.Ordered = append(res.Ordered, Assignment{ID: r.ID, Name: name})
	}
	return res
}

// depths counts, per request, the Wants its Want is built on: a Want that is another one's Want
// plus one of its Methods is one deeper than that one. Shorter Wants go first, so every Want a
// request is built on has its depth by then.
func depths(reqs []Request) map[string]int {
	wanted := make(map[string][]string, len(reqs))
	for _, r := range reqs {
		key := strings.ToLower(r.Want)
		wanted[key] = append(wanted[key], r.ID)
	}

	byLength := slices.Clone(reqs)
	slices.SortStableFunc(byLength, func(a, b Request) int {
		return cmp.Compare(len(strings.ToLower(a.Want)), len(strings.ToLower(b.Want)))
	})
	depth := make(map[string]int, len(reqs))
	for _, r := range byLength {
		for _, s := range r.Methods {
			for _, id := range wanted[strings.ToLower(r.Want+s)] {
				depth[id] = max(depth[id], depth[r.ID]+1)
			}
		}
	}
	return depth
}

// next holds the next number per base, so many clashes on one base stay linear.
func pick(held holders, next map[string]int, r Request) string {
	if _, ok := held.taken(r.Want, r.Methods); !ok {
		return r.Want
	}

	base := r.Want
	if r.Fallback != "" {
		if _, ok := held.taken(r.Fallback, r.Methods); !ok {
			return r.Fallback
		}
		base = r.Fallback
	}

	key := strings.ToLower(base)
	for i := max(next[key], 2); ; i++ {
		name := base + strconv.Itoa(i)
		if _, ok := held.taken(name, r.Methods); !ok {
			next[key] = i + 1
			return name
		}
	}
}
