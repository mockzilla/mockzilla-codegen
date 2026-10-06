// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package runtime holds the helpers generated code imports. It uses the standard library only.
package runtime

// SupportsGeneratorV1 is set while this runtime has everything code from generator API level 1
// uses. Generated files refer to it, so a runtime too old or too new for them fails to compile.
const SupportsGeneratorV1 = true
