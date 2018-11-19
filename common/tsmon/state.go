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

var globalState = NewState()

// GetState returns the State instance held in the context (if set) or else
// returns the global State.
func GetState(ctx context.Context) *State {
	return stateFromContext(ctx)
}

// Callbacks returns all registered Callbacks.
func (s *State) Callbacks() []Callback {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]Callback{}, s.callbacks...)
}

// GlobalCallbacks returns all registered GlobalCallbacks.
func (s *State) GlobalCallbacks() []GlobalCallback {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return append([]GlobalCallback{}, s.globalCallbacks...)
}

// InhibitGlobalCallbacksOnFlush signals that the registered global callbacks
// are not to be executed upon flushing registered metrics.
func (s *State) InhibitGlobalCallbacksOnFlush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.invokeGlobalCallbacksOnFlush = false
}

// InvokeGlobalCallbacksOnFlush signals that the registered global callbacks
// are to be be executed upon flushing registered metrics.
func (s *State) InvokeGlobalCallbacksOnFlush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.invokeGlobalCallbacksOnFlush = true
}

// Monitor returns the State's monitor.
func (s *State) Monitor() monitor.Monitor {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.monitor
}

// RegisterCallbacks registers the given Callback(s) with State.
func (s *State) RegisterCallbacks(f ...Callback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.callbacks = append(s.callbacks, f...)
}

// RegisterGlobalCallbacks registers the given GlobalCallback(s) with State.
func (s *State) RegisterGlobalCallbacks(f ...GlobalCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.globalCallbacks = append(s.globalCallbacks, f...)
}

// Store returns the State's store.
func (s *State) Store() store.Store {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.store
}

// SetMonitor sets the Store's monitor.
func (s *State) SetMonitor(m monitor.Monitor) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.monitor = m
}

// SetStore changes the metric store. All metrics that were registered with
// the old store will be re-registered on the new store.
func (s *State) SetStore(st store.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store = st
}

// ResetCumulativeMetrics resets only cumulative metrics.
func (s *State) ResetCumulativeMetrics(ctx context.Context) {
	store := s.Store()

	registry.Iter(func(m types.Metric) {
		if m.Info().ValueType.IsCumulative() {
			store.Reset(ctx, m)
		}
	})
}

// RunGlobalCallbacks runs all registered global callbacks that produce global
// metrics.
//
// See RegisterGlobalCallback for more info.
func (s *State) RunGlobalCallbacks(ctx context.Context) {
	for _, cb := range s.GlobalCallbacks() {
		cb.Callback(ctx)
	}
}

// Flush sends all the metrics that are registered in the application.
//
// Uses given monitor if not nil, otherwise the State's current monitor.
func (s *State) Flush(ctx context.Context, mon monitor.Monitor) error {
	if mon == nil {
		mon = s.Monitor()
	}

	if mon == nil {
		return errors.New("no tsmon Monitor is configured")
	}

	// Run any callbacks that have been registered to populate values in callback
	// metrics.
	s.runCallbacks(ctx)
	if s.invokeGlobalCallbacksOnFlush {
		s.RunGlobalCallbacks(ctx)
	}

	cells := s.store.GetAll(ctx)
	if len(cells) == 0 {
		return nil
	}

	logging.Debugf(ctx, "Starting tsmon flush: %d cells", len(cells))
	defer logging.Debugf(ctx, "Finished tsmon flush")

	// Split up the payload into chunks if there are too many cells.
	chunkSize := mon.ChunkSize()
	if chunkSize == 0 {
		chunkSize = len(cells)
	}

	var failedSends int
	var lastErr error

	for len(cells) > 0 {
		count := minInt(chunkSize, len(cells))
		if err := mon.Send(ctx, cells[:count]); err != nil {
			logging.Errorf(ctx, "Failed to send %d cells: %v", count, err)
			failedSends += count
			lastErr = err
			// Continue anyway.
		}
		cells = cells[count:]
	}

	logging.Debugf(ctx, "Sent %d/%d cells", len(cells)-failedSends, len(cells))

	s.resetGlobalCallbackMetrics(ctx)

	return lastErr
}

// resetGlobalCallbackMetrics resets metrics produced by global callbacks.
//
// See RegisterGlobalCallback for more info.
func (s *State) resetGlobalCallbackMetrics(ctx context.Context) {
	store := s.Store()

	for _, cb := range s.GlobalCallbacks() {
		for _, m := range cb.Metrics {
			store.Reset(ctx, m)
		}
	}
}

// runCallbacks runs any callbacks that have been registered to populate values
// in callback metrics.
func (s *State) runCallbacks(ctx context.Context) {
	for _, cb := range s.Callbacks() {
		cb(ctx)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
