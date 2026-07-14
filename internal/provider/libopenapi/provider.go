// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package libopenapi implements provider.Provider and is the only importer of libopenapi.
package libopenapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/bundler"
	"github.com/pb33f/libopenapi/datamodel"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/oasdoc"
	"github.com/mockzilla/codegen/internal/provider"
	"github.com/mockzilla/codegen/internal/spec"
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
	if err := ctx.Err(); err != nil {
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

// Bundle returns the input untouched when every $ref is local.
func (p *Provider) Bundle(ctx context.Context, src provider.Source) (out *provider.Bundled, err error) {
	defer recoverPanic(&err, src.Path)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	doc, err := oasdoc.Parse(src.Data, src.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", provider.ErrBundle, err)
	}
	if !hasExternalRef(doc.Refs()) {
		return &provider.Bundled{Data: src.Data}, nil
	}

	res, err := bundler.BundleBytesComposedWithOrigins(src.Data, p.bundleConfig(src), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", provider.ErrBundle, src.Path, err)
	}

	origins := make(map[string]diag.Origin, len(res.Origins))
	for ref, o := range res.Origins {
		origins[strings.TrimPrefix(ref, "#")] = diag.Origin{File: o.OriginalFile, Line: o.Line, Col: o.Column}
	}
	return &provider.Bundled{Data: res.Bytes, Origins: origins}, nil
}

func (p *Provider) Parse(ctx context.Context, data []byte, opts provider.ParseOptions) (doc *spec.Document, diags []diag.Diagnostic, err error) {
	defer recoverPanic(&err, opts.File)
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	d, err := libopenapi.NewDocumentWithConfiguration(data, p.config())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %w", provider.ErrParse, opts.File, err)
	}
	version, err := specVersion(d.GetSpecInfo())
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", err, opts.File)
	}

	model, buildErr := d.BuildV3Model()
	if model == nil {
		return nil, nil, fmt.Errorf("%w: %s: %w", provider.ErrParse, opts.File, buildErr)
	}

	c := newConverter(version, opts)
	c.buildIssues(buildErr, model.Index.GetCircularReferences())
	doc = c.document(&model.Model)
	return doc, c.diags.List(), nil
}

func (p *Provider) config() *datamodel.DocumentConfiguration {
	return &datamodel.DocumentConfiguration{
		Logger:               p.logger,
		TransformSiblingRefs: true,
	}
}

// libopenapi's local file system reads only the files refs name, so no file filter is needed.
func (p *Provider) bundleConfig(src provider.Source) *datamodel.DocumentConfiguration {
	cfg := p.config()
	cfg.AllowFileReferences = true
	cfg.AllowRemoteReferences = src.AllowRemote
	cfg.ExtractRefsSequentially = true

	base, err := url.Parse(src.Path)
	switch {
	case err == nil && (base.Scheme == "http" || base.Scheme == "https"):
		base.Path = path.Dir(base.Path)
		cfg.BaseURL = base
	case src.Path != "":
		cfg.SpecFilePath = src.Path
		cfg.BasePath = filepath.Dir(src.Path)
	}
	if src.BaseDir != "" {
		cfg.BasePath = src.BaseDir
	}
	return cfg
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

func hasExternalRef(refs []oasdoc.Ref) bool {
	for _, r := range refs {
		if !strings.HasPrefix(r.Value, "#") {
			return true
		}
	}
	return false
}

func recoverPanic(err *error, file string) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%w: %s: %v", provider.ErrProviderPanic, file, r)
	}
}
