// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"

	cfgcachepb "go.chromium.org/luci/server/cfgcache/proto"
)

// stateRecord is a mutex-protected [stateRecordSnapshot].
type stateRecord struct {
	mu sync.Mutex

	snap stateRecordSnapshot
}

// stateRecordSnapshot is a quick snapshot of stateRecord.
//
// Used for tests and the status page.
type stateRecordSnapshot struct {
	// lastRefreshErr is the error from lastRefreshAttempt, or nil if it
	// succeeded.
	lastRefreshErr error

	// acceptedState maps each config.Set to the Meta currently loaded into
	// memory.
	acceptedState map[config.Set]*cfgcachepb.Meta

	// configSetErrors maps a config.Set to the error encountered on the most
	// recent refresh attempt (e.g. version incompatibility or ToMemory failure).
	configSetErrors map[config.Set]error

	// lastRefreshed indicates the time of the last successful refresh.
	lastRefreshed time.Time

	// lastRefreshAttempt indicates the last refresh attempt (successful or not).
	lastRefreshAttempt time.Time
}

func (s *stateRecordSnapshot) clone() *stateRecordSnapshot {
	return &stateRecordSnapshot{
		lastRefreshErr:     s.lastRefreshErr,
		acceptedState:      maps.Clone(s.acceptedState),
		configSetErrors:    maps.Clone(s.configSetErrors),
		lastRefreshed:      s.lastRefreshed,
		lastRefreshAttempt: s.lastRefreshAttempt,
	}
}

// acceptedState locks the state and returns a cloned snapshot of
// `acceptedState`.
//
// Never returns `nil`.
func (s *stateRecord) acceptedState() map[config.Set]*cfgcachepb.Meta {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snap.acceptedState == nil {
		return map[config.Set]*cfgcachepb.Meta{}
	}
	return maps.Clone(s.snap.acceptedState)
}

// snapshot locks the state and returns a cloned snapshot from `s`.
func (s *stateRecord) snapshot() *stateRecordSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.clone()
}

// record records a refresh attempt.
//
// Takes ownership of `accepted` and `csErrs`.
func (s *stateRecord) record(ctx context.Context, err error, accepted map[config.Set]*cfgcachepb.Meta, csErrs map[config.Set]error) {
	now := clock.Now(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snap.acceptedState = accepted
	s.snap.configSetErrors = csErrs

	s.snap.lastRefreshAttempt = now
	s.snap.lastRefreshErr = err
	if err == nil {
		s.snap.lastRefreshed = now
	}
}

// checkStale returns nil if the current accepted state is not older than
// maxStaleness ago, otherwise returns an error.
func (s *stateRecord) checkStale(ctx context.Context, maxStaleness time.Duration) error {
	now := clock.Now(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snap.lastRefreshed.IsZero() {
		return errors.New("config was never refreshed")
	}

	var err error
	if age := now.Sub(s.snap.lastRefreshed); age > maxStaleness {
		err = cmp.Or(s.snap.lastRefreshErr, errors.New("no refresh error?"))
		err = fmt.Errorf("config is stale (%s > %s): %w", age, maxStaleness, err)
	}

	return err
}

// checkFresh checks that the cached in-memory value was refreshed sometime
// more recently than [Registry.MaxMemoryStaleness] ago.
//
// If it wasn't, then this returns an error.
func (ic *ingestConfig) checkFresh(ctx context.Context) error {
	ic.checkInitialized()
	if err := ic.state.checkStale(ctx, ic.reg.MaxMemoryStaleness); err != nil {
		return fmt.Errorf("cfgcache pipeline %q: %w", ic, err)
	}
	return nil
}

// checkStoredVersionCompatible checks to see if `meta`'s stored transform
// version is compatible with the toMemoryMaxVersion/toMemoryMinVersion of
// our ToMemory function.
func (ic *ingestConfig) checkStoredVersionCompatible(cs config.Set, meta *cfgcachepb.Meta) error {
	if mver := ic.toMemoryMaxVersion; meta.TransformVersion > mver {
		return errors.Fmt("%s[%s]: transform version is %d > max(%d)", ic.id, cs, meta.TransformVersion, mver)
	}
	if mver := ic.toMemoryMinVersion; meta.TransformVersion < mver {
		return errors.Fmt("%s[%s]: transform version is %d < min(%d)", ic.id, cs, meta.TransformVersion, mver)
	}
	return nil
}

// refresh actually refreshes from storage into our in-memory cache.
func (ic *ingestConfig) refresh(ctx context.Context) (err error) {
	ctx, cancel := clock.WithTimeout(ctx, ic.reg.MemoryRefreshTimeout)
	defer cancel()

	ctx = logging.SetField(ctx, "pipeline", ic.id)

	accepted := ic.state.acceptedState()
	var csErrs map[config.Set]error
	setCsErr := func(cs config.Set, err error) {
		if err == nil {
			return
		}
		if csErrs == nil {
			csErrs = map[config.Set]error{cs: err}
		} else {
			csErrs[cs] = err
		}
	}

	defer func() {
		// Log all errors, ignoring context cancellation.
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				logging.Errorf(ctx, "%s", err)
			}
		}
		for cs, err := range csErrs {
			if !errors.Is(err, context.Canceled) {
				(logging.Fields{"config.Set": cs}).Errorf(ctx, "%s", err)
			}
		}
		ic.state.record(ctx, err, accepted, csErrs)
	}()

	dbState, err := ic.store.LoadMeta(ctx, ic.id)
	if err != nil {
		return errors.Fmt("loading by id: %w", err)
	}
	toGet := map[config.Set]*cfgcachepb.Meta{}
	for cs, meta := range dbState {
		if accepted[cs] != meta {
			if err := ic.checkStoredVersionCompatible(cs, meta); err != nil {
				setCsErr(cs, err)
			} else {
				toGet[cs] = meta
			}
		}
	}

	if len(toGet) > 0 {
		yielded := make(map[config.Set]struct{}, len(toGet))
		for sc, err := range ic.store.Get(ctx, ic.id, toGet) {
			if err != nil {
				if sc == nil {
					return errors.Fmt("fetching: general error: %w", err)
				}
				return errors.Fmt("fetching: %s: %w", sc.ConfigSet, err)
			}

			ctx := logging.SetField(ctx, "config.Set", sc.ConfigSet)

			if _, ok := toGet[sc.ConfigSet]; !ok {
				return errors.Fmt("fetching: %s: unrequested config set", sc.ConfigSet)
			}
			if _, ok := yielded[sc.ConfigSet]; ok {
				return errors.Fmt("fetching: %s: duplicate config set", sc.ConfigSet)
			}
			yielded[sc.ConfigSet] = struct{}{}

			// Refresh dbState since we do use it below (even though we discard
			// the value). `sc.Meta` may be different from what we got in
			// LoadMeta because of eventual consistency in the database.
			dbState[sc.ConfigSet] = sc.Meta
			if accepted[sc.ConfigSet] == sc.Meta {
				continue
			}
			if err := ic.checkStoredVersionCompatible(sc.ConfigSet, sc.Meta); err != nil {
				setCsErr(sc.ConfigSet, err)
				continue
			}

			// Send it to our changed function and record that it's now accepted.
			if err := ic.toMemory(ctx, sc.ConfigSet, sc.Config); err != nil {
				csErr := errors.Fmt("consuming %s: %w", sc.ConfigSet, err)
				setCsErr(sc.ConfigSet, csErr)
			} else {
				accepted[sc.ConfigSet] = sc.Meta
			}
		}
		for cs := range toGet {
			if _, ok := yielded[cs]; !ok {
				delete(dbState, cs)
			}
		}
	}

	// For any items in accepted not in dbState, clear them from memory.
	for cs := range accepted {
		ctx := logging.SetField(ctx, "config.Set", cs)

		if _, has := dbState[cs]; !has {
			_ = ic.toMemory(ctx, cs, nil)
			delete(accepted, cs)
		}
	}

	return nil
}
