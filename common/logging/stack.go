// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"context"
)

// SetStackTrace returns a new context with the given stack trace set in it.
//
// All messages logged through it will be associated with this stack trace.
// This must be a stack trace in a format compatible with what is produced by
// https://pkg.go.dev/runtime/debug#Stack.
//
// This is an advanced function. If you just want to log an error with
// a stack trace attached, use
func SetStackTrace(ctx context.Context, stack StackTrace) context.Context {
	return modifyCtx(ctx, func(lc *LogContext) { lc.StackTrace = stack })
}

// ErrorWithStackTrace submits an error with a stack trace associated with it.
func ErrorWithStackTrace(ctx context.Context, stack StackTrace, fmt string, args ...any) {
	Errorf(SetStackTrace(ctx, stack), fmt, args...)
}
