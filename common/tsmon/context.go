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
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
)

// WithState returns a new context holding the given State instance.
func WithState(c context.Context, s *State) context.Context {
	return context.WithValue(c, stateKey, s)
}

// WithFakes returns a new context holding a new State with a fake store and a
// fake monitor.
func WithFakes(c context.Context) (context.Context, *store.Fake, *monitor.Fake) {
	s := &store.Fake{}
	m := &monitor.Fake{}
	return WithState(c, &State{
		store:                        s,
		monitor:                      m,
		invokeGlobalCallbacksOnFlush: true,
	}), s, m
}

// WithDummyInMemory returns a new context holding a new State with a new in-
// memory store and a fake monitor.
func WithDummyInMemory(c context.Context) (context.Context, *monitor.Fake) {
	m := &monitor.Fake{}
	return WithState(c, &State{
		store:                        store.NewInMemory(&target.Task{}),
		monitor:                      m,
		invokeGlobalCallbacksOnFlush: true,
	}), m
}

type key int

var stateKey key = 1
