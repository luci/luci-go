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
	"errors"
	"sync"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/types"
)

// State holds the configuration of the tsmon library. There is one global
// instance of State, but it can be overridden in a Context by tests.
type State struct {
	mu                           sync.RWMutex
	store                        store.Store
	monitor                      monitor.Monitor
	flusher                      *autoFlusher
	callbacks                    []Callback
	globalCallbacks              []GlobalCallback
	invokeGlobalCallbacksOnFlush bool
}

// NewState returns a new State instance, configured with a nil store and nil
// monitor. By default, global callbacks that are registered will be invoked
// when flushing registered metrics.
func NewState() *State {
	return &State{
		store:                        store.NewNilStore(),
		monitor:                      monitor.NewNilMonitor(),
		invokeGlobalCallbacksOnFlush: true,
	}
}

// GetState returns the State instance held in the context (if set) or else
// returns the global state.
func GetState(c context.Context) *State {
	if ret := c.Value(stateKey); ret != nil {
		return ret.(*State)
	}
	return globalState
}

var globalState = NewState()

// InhibitGlobalCallbacksOnFlush signals that the registered global callbacks
// are not to be executed upon flushing registered metrics.
func (state *State) InhibitGlobalCallbacksOnFlush() {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.invokeGlobalCallbacksOnFlush = false
}

// InvokeGlobalCallbacksOnFlush signals that the registered global callbacks
// are to be be executed upon flushing registered metrics.
func (state *State) InvokeGlobalCallbacksOnFlush() {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.invokeGlobalCallbacksOnFlush = true
}

// Monitor returns the State's monitor.
func (state *State) Monitor() monitor.Monitor {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.monitor
}

// Store returns the State's store.
func (state *State) Store() store.Store {
	state.mu.RLock()
	defer state.mu.RUnlock()

	return state.store
}

// SetMonitor sets the Store's monitor.
func (state *State) SetMonitor(m monitor.Monitor) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.monitor = m
}

// SetStore changes the metric store. All metrics that were registered with
// the old store will be re-registered on the new store.
func (state *State) SetStore(s store.Store) {
	state.mu.Lock()
	defer state.mu.Unlock()

	state.store = s
}

// ResetCumulativeMetrics resets only cumulative metrics.
func (state *State) ResetCumulativeMetrics(c context.Context) {
	registry.Iter(func(m types.Metric) {
		if m.Info().ValueType.IsCumulative() {
			state.store.Reset(c, m)
		}
	})
}

// RunGlobalCallbacks runs all registered global callbacks that produce global
// metrics.
//
// See RegisterGlobalCallback for more info.
func (state *State) RunGlobalCallbacks(c context.Context) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, gcp := range state.globalCallbacks {
		gcp.Callback(c)
	}
}

// Flush sends all the metrics that are registered in the application.
//
// Uses given monitor if not nil, otherwise the State's current monitor.
func (state *State) Flush(c context.Context, mon monitor.Monitor) error {
	if mon == nil {
		// Prevent the monitor from being changed between now and the time metrics
		// are sent.
		state.mu.RLock()
		defer state.mu.RUnlock()

		mon = state.monitor
	}

	if mon == nil {
		return errors.New("no tsmon Monitor is configured")
	}

	// Run any callbacks that have been registered to populate values in callback
	// metrics.
	state.runCallbacks(c)
	if state.invokeGlobalCallbacksOnFlush {
		state.RunGlobalCallbacks(c)
	}

	cells := state.store.GetAll(c)
	if len(cells) == 0 {
		return nil
	}

	logging.Debugf(c, "Starting tsmon flush: %d cells", len(cells))
	defer logging.Debugf(c, "Finished tsmon flush")

	// Split up the payload into chunks if there are too many cells.
	chunkSize := mon.ChunkSize()
	if chunkSize == 0 {
		chunkSize = len(cells)
	}

	sent := 0 // TODO(slewiskelly): This isn't used?
	var lastErr error
	for len(cells) > 0 {
		count := minInt(chunkSize, len(cells))
		if err := mon.Send(c, cells[:count]); err != nil {
			logging.Errorf(c, "Failed to send %d cells: %v", count, err)
			lastErr = err
			// Continue anyway.
		}
		cells = cells[count:]
		sent += count
	}

	state.resetGlobalCallbackMetrics(c)

	return lastErr
}

// resetGlobalCallbackMetrics resets metrics produced by global callbacks.
//
// See RegisterGlobalCallback for more info.
func (state *State) resetGlobalCallbackMetrics(c context.Context) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, gcp := range state.globalCallbacks {
		for _, m := range gcp.Metrics {
			state.store.Reset(c, m)
		}
	}
}

// runCallbacks runs any callbacks that have been registered to populate values
// in callback metrics.
func (state *State) runCallbacks(c context.Context) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, f := range state.callbacks {
		f(c)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
