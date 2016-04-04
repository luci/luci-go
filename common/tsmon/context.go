// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/tsmon/monitor"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
)

// State holds the configuration of the tsmon library.  There is one global
// instance of State, but it can be overridden in a Context by tests.
type State struct {
	S       store.Store
	M       monitor.Monitor
	Flusher *autoFlusher

	RegisteredMetrics     map[string]types.Metric
	RegisteredMetricsLock sync.RWMutex

	CallbacksMutex  sync.RWMutex
	Callbacks       []Callback
	GlobalCallbacks []GlobalCallback
}

// A GlobalCallback is a Callback with the list of metrics it affects, so those
// metrics can be reset after they are flushed.
type GlobalCallback struct {
	Callback
	Metrics []types.Metric
}

// GetState returns the State instance held in the context (if set) or else
// returns the global state.
func GetState(c context.Context) *State {
	if ret := c.Value(stateKey); ret != nil {
		return ret.(*State)
	}
	return globalState
}

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
		S:                 s,
		M:                 m,
		RegisteredMetrics: map[string]types.Metric{},
	}), s, m
}

// WithDummyInMemory returns a new context holding a new State with a new in-
// memory store and a fake monitor.
func WithDummyInMemory(c context.Context) (context.Context, *monitor.Fake) {
	s := store.NewInMemory(&target.Task{})
	m := &monitor.Fake{}
	return WithState(c, &State{
		S:                 s,
		M:                 m,
		RegisteredMetrics: map[string]types.Metric{},
	}), m
}

type key int

var (
	globalState = &State{
		S:                 store.NewNilStore(),
		M:                 monitor.NewNilMonitor(),
		RegisteredMetrics: map[string]types.Metric{},
	}
	stateKey key = 1
)
