// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

// SupportsGeneratorV2 is set while this runtime has everything code from generator API level 2
// uses. Generated files refer to it, so a runtime too old or too new for them fails to compile.
const SupportsGeneratorV2 = true
