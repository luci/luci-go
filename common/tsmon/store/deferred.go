// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package store

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// DeferredStore collects modifications made after Start is called, and applies
// them all at once with a single call to ModifyMulti in Finalize.
type DeferredStore struct {
	Store
}

type contextKey int

const contextStateKey contextKey = iota

type contextState struct {
	mods []Modification
	lock sync.Mutex
}

// NewDeferred creates a new deferred store that wraps the given metric store.
func NewDeferred(baseStore Store) *DeferredStore {
	return &DeferredStore{baseStore}
}

// Set stores a modification in the context to be applied when Finalize is
// called.  If Start has not been called yet this logs a warning and calls the
// base store's Set method immediately.
func (s *DeferredStore) Set(ctx context.Context, h types.Metric, resetTime time.Time, fieldVals []interface{}, value interface{}) error {
	state := s.state(ctx)
	if state == nil {
		logging.Warningf(ctx, "DeferredStore used before Start() was called")
		return s.Store.Set(ctx, h, resetTime, fieldVals, value)
	}

	state.lock.Lock()
	defer state.lock.Unlock()
	state.mods = append(state.mods, Modification{
		Metric:    h,
		ResetTime: resetTime,
		FieldVals: fieldVals,
		SetValue:  value,
	})
	return nil
}

// Incr stores a modification in the context to be applied when Finalize is
// called.  If Start has not been called yet this logs a warning and calls the
// base store's Incr method immediately.
func (s *DeferredStore) Incr(ctx context.Context, h types.Metric, resetTime time.Time, fieldVals []interface{}, delta interface{}) error {
	state := s.state(ctx)
	if state == nil {
		logging.Warningf(ctx, "DeferredStore used before Start() was called")
		return s.Store.Incr(ctx, h, resetTime, fieldVals, delta)
	}

	state.lock.Lock()
	defer state.lock.Unlock()
	state.mods = append(state.mods, Modification{
		Metric:    h,
		ResetTime: resetTime,
		FieldVals: fieldVals,
		IncrDelta: delta,
	})
	return nil
}

// ModifyMulti adds all the modifications to the context to be applied when
// Finalize is called.  If Start has not been called yet this logs a warning and
// calls the base store's ModifyMulti method immediately.
func (s *DeferredStore) ModifyMulti(ctx context.Context, mods []Modification) error {
	state := s.state(ctx)
	if state == nil {
		logging.Warningf(ctx, "DeferredStore used before Start() was called")
		return s.Store.ModifyMulti(ctx, mods)
	}

	state.lock.Lock()
	defer state.lock.Unlock()
	state.mods = append(state.mods, mods...)
	return nil
}

func (s *DeferredStore) state(ctx context.Context) *contextState {
	if ret, ok := ctx.Value(contextStateKey).(*contextState); ok {
		return ret
	}
	return nil
}

// Start causes subsequent Set and Incr calls to be deferred until Finalize is
// called.
func (s *DeferredStore) Start(ctx context.Context) context.Context {
	if s.state(ctx) != nil {
		logging.Warningf(ctx, "DeferredStore.Start called twice on the same context")
	}
	return context.WithValue(ctx, contextStateKey, &contextState{})
}

// Finalize applies all the modifications made since Start was called.
func (s *DeferredStore) Finalize(ctx context.Context) error {
	state := s.state(ctx)
	if state == nil {
		logging.Warningf(ctx, "DeferredStore.Finalize called before Start")
		return nil
	}

	state.lock.Lock()
	defer state.lock.Unlock()
	err := s.Store.ModifyMulti(ctx, state.mods)
	state.mods = nil
	return err
}
