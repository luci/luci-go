// Copyright 2020 The LUCI Authors.
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

package internal

import (
	"context"
	"time"
)

// MergedContext is a context that inherits everything from Root, except values
// are additionally inherited from Fallback.
type MergedContext struct {
	Root     context.Context
	Fallback context.Context
}

var _ context.Context = (*MergedContext)(nil)

func (mc *MergedContext) Value(key any) any {
	val := mc.Root.Value(key)
	if val == nil {
		val = mc.Fallback.Value(key)
	}
	return val
}

func (mc *MergedContext) Deadline() (time.Time, bool) { return mc.Root.Deadline() }
func (mc *MergedContext) Done() <-chan struct{}       { return mc.Root.Done() }
func (mc *MergedContext) Err() error                  { return mc.Root.Err() }
