// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package itest

import (
	"fmt"
	"strings"
)

// Known is a failure the known-failures file expects for a spec.
type Known struct {
	Stage  string
	Reason string
}

// Verdict is how a run compares with the known failures: failures the list does not expect, and
// listed specs that passed.
type Verdict struct {
	New   []Result
	Fixed []string
}

// ParseKnown reads a known-failures file: one "<spec> <stage>: <reason>" per line, blank lines and
// lines starting with # left out.
func ParseKnown(data []byte) (map[string]Known, error) {
	known := map[string]Known{}
	n := 0
	for line := range strings.Lines(string(data)) {
		n++
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, rest, _ := strings.Cut(line, " ")
		stage, reason, _ := strings.Cut(rest, ":")
		reason = strings.TrimSpace(reason)
		_, isListed := known[name]
		switch {
		case stage != StageGenerate && stage != StageBuild || reason == "":
			return nil, fmt.Errorf("%w %d: want \"<spec> <stage>: <reason>\" with stage %s or %s, got %q",
				ErrKnownLine, n, StageGenerate, StageBuild, line)
		case isListed:
			return nil, fmt.Errorf("%w %d: %s is listed twice", ErrKnownLine, n, name)
		}
		known[name] = Known{Stage: stage, Reason: reason}
	}
	return known, nil
}

// Compare returns the verdict of results against known. A listed spec that fails at another stage
// is a new failure. Listed specs the run left out are neither new nor fixed.
func Compare(results []Result, known map[string]Known) Verdict {
	var v Verdict
	for _, r := range results {
		_, isListed := known[r.Job.Spec.Name]
		switch {
		case r.Stage == "" && isListed:
			v.Fixed = append(v.Fixed, r.Job.Spec.Name)
		case isNew(r, known):
			v.New = append(v.New, r)
		}
	}
	return v
}

func isNew(r Result, known map[string]Known) bool {
	k, isListed := known[r.Job.Spec.Name]
	return r.Stage != "" && (!isListed || k.Stage != r.Stage)
}
