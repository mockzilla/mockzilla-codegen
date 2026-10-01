// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package diag

const (
	CodeCircularRef         = "circular-ref"
	CodeInfiniteCircularRef = "infinite-circular-ref"
	CodeBuildIssue          = "build-issue"
	CodeSchemaBuild         = "schema-build"
	CodeUnknownType         = "unknown-type"
	CodeInvalidStatus       = "invalid-status"
	CodeUnresolvedMapping   = "unresolved-mapping"
	CodeOptionalPathParam   = "optional-path-param"
	CodeDuplicateParam      = "duplicate-param"
	CodeOverlayTarget       = "overlay-target"
	CodeBundleRename        = "bundle-rename"
	CodeUnbundledRef        = "unbundled-ref"
	CodeFilterUnknown       = "filter-unknown"
	CodeFilterRequired      = "filter-required"
	CodeFilterEmpty         = "filter-empty"
	CodePruneUnsupported    = "prune-unsupported"
	CodePruneSkipped        = "prune-skipped"
	CodeNameClash           = "name-clash"
	CodeUnionDuplicate      = "union-duplicate"
	CodeUnionSelf           = "union-self"
	CodePatternUnsupported  = "pattern-unsupported"
	CodeErrorMapping        = "error-mapping"
	CodeExtensionUnknown    = "extension-unknown"
	CodeExtensionValue      = "extension-value"
	CodeEnumIgnored         = "enum-ignored"
	CodeEnumValue           = "enum-value"
	CodeAllOfConflict       = "allof-conflict"
	CodeAllOfCycle          = "allof-cycle"
	CodeAliasCycle          = "alias-cycle"
	CodeRouteDropped        = "route-dropped"
	CodeStreamOnly          = "stream-only"
	CodeMCPToolName         = "mcp-tool-name"
	CodeImportUnused        = "import-unused"
)
