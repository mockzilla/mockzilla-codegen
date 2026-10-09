// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package libopenapi implements provider.Provider and is the only importer of libopenapi.
package libopenapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	lowv3 "github.com/pb33f/libopenapi/datamodel/low/v3"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/spec"
)

var _ provider.Provider = (*Provider)(nil)

// Provider reads specs with libopenapi. Its logs go nowhere unless WithDebug is set.
type Provider struct {
	logger *slog.Logger
}

func New(opts ...Option) *Provider {
	p := &Provider{logger: slog.New(slog.DiscardHandler)}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *Provider) ApplyOverlay(ctx context.Context, data, overlay []byte) (out []byte, diags []diag.Diagnostic, err error) {
	defer recoverPanic(&err, "overlay")
	if err = ctx.Err(); err != nil {
		return nil, nil, err
	}

	res, err := libopenapi.ApplyOverlayFromBytesToSpecBytes(data, overlay)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", provider.ErrOverlay, err)
	}

	var c diag.Collector
	for _, w := range res.Warnings {
		c.Append(diag.Diagnostic{
			Severity: diag.Warning,
			Code:     diag.CodeOverlayTarget,
			Message:  "overlay target " + w.Target + ": " + w.Message,
		})
	}
	return res.Bytes, c.List(), nil
}

func (p *Provider) Parse(ctx context.Context, data []byte, opts provider.ParseOptions) (doc *spec.Document, diags []diag.Diagnostic, err error) {
	defer recoverPanic(&err, opts.File)
	if err = ctx.Err(); err != nil {
		return nil, nil, err
	}

	cfg := p.config()
	d, err := libopenapi.NewDocumentWithConfiguration(data, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %w", provider.ErrParse, syntaxAt(opts.File, data, err), err)
	}
	version, err := specVersion(d.GetSpecInfo())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", err, opts.File)
	}

	// BuildV3Model drops the whole model for one ref it cannot follow, like one inside an example value.
	low, buildErr := lowv3.CreateDocumentFromConfig(d.GetSpecInfo(), cfg)
	c := newConverter(version, opts)
	c.buildIssues(buildErr, low.Index.GetCircularReferences())
	doc = c.document(v3.NewDocument(low))
	return doc, c.diags.List(), nil
}

func (p *Provider) config() *datamodel.DocumentConfiguration {
	return &datamodel.DocumentConfiguration{
		Logger:               p.logger,
		TransformSiblingRefs: true,
	}
}

func specVersion(info *datamodel.SpecInfo) (spec.Version, error) {
	switch info.SpecFormat {
	case datamodel.OAS3:
		return spec.V30, nil
	case datamodel.OAS31:
		return spec.V31, nil
	case datamodel.OAS32:
		return spec.V32, nil
	default:
		return 0, fmt.Errorf("%w: %s", provider.ErrUnsupportedVersion, info.Version)
	}
}

// syntaxAt is file with the line and column of a JSON syntax error in data, from its byte offset.
func syntaxAt(file string, data []byte, err error) string {
	var se *json.SyntaxError
	if !errors.As(err, &se) || se.Offset < 1 || se.Offset > int64(len(data)) {
		return file
	}

	read := data[:se.Offset]
	line := bytes.Count(read, []byte("\n")) + 1
	col := len(read) - 1 - bytes.LastIndexByte(read, '\n')
	return file + ":" + strconv.Itoa(line) + ":" + strconv.Itoa(col)
}

func recoverPanic(err *error, file string) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%w: %s: %v", provider.ErrProviderPanic, file, r)
	}
}
