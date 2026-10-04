// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import "context"

type operationIDKey struct{}

// WithOperationID returns a copy of ctx that holds the name of the operation being called.
func WithOperationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, operationIDKey{}, id)
}

// OperationID returns the operation name ctx holds, or "" when it holds none.
func OperationID(ctx context.Context) string {
	id, _ := ctx.Value(operationIDKey{}).(string)
	return id
}
