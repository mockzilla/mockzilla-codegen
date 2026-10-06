// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type WriteAction int

const (
	ActionWrite WriteAction = iota
	ActionSkip
)

// WriteOptions control Write. DryRun reports what Write would do and writes nothing.
type WriteOptions struct {
	DryRun bool
}

// WriteReport says what Write did with one file.
type WriteReport struct {
	Path   string
	Action WriteAction
}

func (a WriteAction) String() string {
	switch a {
	case ActionWrite:
		return "write"
	case ActionSkip:
		return "skip"
	default:
		return "unknown"
	}
}

// Write writes the files of res, making folders as needed. A scaffold that exists is skipped
// unless its IsOverwritten is set. It stops at the first failure and returns the reports of the
// files before it.
func Write(res *Result, opts WriteOptions) ([]WriteReport, error) {
	reports := make([]WriteReport, 0, len(res.Files))
	for _, f := range res.Files {
		action, err := actionFor(f)
		if err == nil && action == ActionWrite && !opts.DryRun {
			err = writeFile(f)
		}
		if err != nil {
			return reports, fmt.Errorf("%w: %w", ErrWrite, err)
		}
		reports = append(reports, WriteReport{Path: f.Path, Action: action})
	}
	return reports, nil
}

func actionFor(f File) (WriteAction, error) {
	if f.Kind != FileScaffold || f.IsOverwritten {
		return ActionWrite, nil
	}

	_, err := os.Stat(f.Path)
	switch {
	case err == nil:
		return ActionSkip, nil
	case errors.Is(err, fs.ErrNotExist):
		return ActionWrite, nil
	default:
		return ActionWrite, err
	}
}

func writeFile(f File) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.Path, f.Content, 0o644)
}
