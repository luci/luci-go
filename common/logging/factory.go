// Copyright 2015 The LUCI Authors.
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

// Factory returns a Logger instance bound to the specified log context.
type Factory func(context.Context, *LogContext) Logger

// SetFactory sets the Logger factory for this context.
//
// The factory will be called each time Get(context) is used.
func SetFactory(ctx context.Context, f Factory) context.Context {
	return modifyCtx(ctx, func(lc *LogContext) { lc.Factory = f })
}

// GetFactory returns the currently-configured logging factory (or nil).
func GetFactory(ctx context.Context) Factory {
	return readCtx(ctx).Factory
}

// Get the current Logger, or a logger that ignores all messages if none
// is defined.
func Get(ctx context.Context) Logger {
	if lc := readCtx(ctx); lc.Factory != nil {
		return lc.Factory(ctx, lc)
	}
	return Null
}
