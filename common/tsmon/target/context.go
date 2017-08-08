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

package target

import (
	"go.chromium.org/luci/common/tsmon/types"
	"golang.org/x/net/context"
)

type key int

const targetKey key = iota

// Set returns a new context with the given target set.  If this context is
// passed to metric Set, Get or Incr methods the metrics for that target will be
// affected.  A nil target means to use the default target.
func Set(ctx context.Context, t types.Target) context.Context {
	return context.WithValue(ctx, targetKey, t)
}

// Get returns the target set in this context.
func Get(ctx context.Context) types.Target {
	if t, ok := ctx.Value(targetKey).(types.Target); ok {
		return t
	}
	return nil
}

// GetWithDefault is like Get, except it returns the given default value if
// there is no target set in the context.
func GetWithDefault(ctx context.Context, def types.Target) types.Target {
	if t := Get(ctx); t != nil {
		return t
	}
	return def
}
