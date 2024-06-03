// Copyright 2016 The LUCI Authors.
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

package tsmon

import (
	"context"

	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
)

// WithState returns a new context holding the given State instance.
func WithState(ctx context.Context, s *State) context.Context {
	return context.WithValue(ctx, stateKey, s)
}

// WithFakes returns a new context holding a new State with a fake store and a
// fake monitor.
func WithFakes(ctx context.Context) (context.Context, *store.Fake, *monitor.Fake) {
	s := &store.Fake{}
	m := &monitor.Fake{}
	return WithState(ctx, &State{
		store:                        s,
		monitor:                      m,
		invokeGlobalCallbacksOnFlush: 1,
		registry:                     registry.Global,
	}), s, m
}

// WithDummyInMemory returns a new context holding a new State with a new in-
// memory store and a fake monitor.
func WithDummyInMemory(ctx context.Context) (context.Context, *monitor.Fake) {
	m := &monitor.Fake{}
	return WithState(ctx, &State{
		store:                        store.NewInMemory(&target.Task{}),
		monitor:                      m,
		invokeGlobalCallbacksOnFlush: 1,
		registry:                     registry.Global,
	}), m
}

// stateFromContext returns a State instance from the given context, if there
// is one; otherwise the default State is returned.
func stateFromContext(ctx context.Context) *State {
	if ret := ctx.Value(stateKey); ret != nil {
		return ret.(*State)
	}

	return globalState
}

type key string

var stateKey = key("tsmon.State")
