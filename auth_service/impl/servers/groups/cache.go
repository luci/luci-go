// Copyright 2024 The LUCI Authors.
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

package groups

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/impl/model"
)

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

type AuthGroupsSnapshot struct {
	authDBRev int64
	groups    []*model.AuthGroup
}

type CachingGroupsProvider struct {
	m      sync.RWMutex
	cached *AuthGroupsSnapshot
}

func (cgp *CachingGroupsProvider) GetAllAuthGroups(ctx context.Context) (groups []*model.AuthGroup, err error) {
	// Grab the latest AuthDB revision number.
	latestState, err := model.GetReplicationState(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to check the latest AuthDB revision").Err()
	}

	// Use the cached snapshot if it's up-to-date.
	cgp.m.RLock()
	cached := cgp.cached
	cgp.m.RUnlock()
	if cached != nil && cached.authDBRev == latestState.AuthDBRev {
		logging.Debugf(ctx, "Using cached AuthGroups from rev %d", cached.authDBRev)
		return cached.groups, nil
	}

	if cached == nil {
		logging.Infof(ctx, "Initializing AuthGroups cache")
	} else {
		logging.Infof(ctx,
			"Refreshing AuthGroups (have groups from rev %d, want rev >= %d)",
			cached.authDBRev, latestState.AuthDBRev)
	}

	// Fetch all groups from Datastore. Make all other callers of GetAllAuthGroups
	// wait to avoid all of them hitting the Datastore at once when the cached
	// groups become stale.
	cgp.m.Lock()
	defer cgp.m.Unlock()

	// Maybe the groups were already fetched while we were waiting on the lock.
	if cgp.cached != nil && cgp.cached.authDBRev >= latestState.AuthDBRev {
		logging.Infof(ctx, "Other goroutine fetched AuthGroups for rev %d already",
			cgp.cached.authDBRev)
		return cgp.cached.groups, nil
	}

	logging.Infof(ctx, "Fetching AuthGroups from Datastore")

	// Transactionally fetch all groups.
	snap, err := takeGroupsSnapshot(ctx)
	if err != nil {
		logging.Errorf(ctx, err.Error())
		return []*model.AuthGroup{}, err
	}

	cgp.cached = snap
	logging.Infof(ctx, "Refreshed AuthGroups (rev %d)", cgp.cached.authDBRev)
	return cgp.cached.groups, nil
}

// RefreshPeriodically runs a loop that periodically refreshes the cached
// snapshot of AuthGroups.
func (cgp *CachingGroupsProvider) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, 30*time.Second); r.Err != nil {
			return // the context is canceled
		}
		if _, err := cgp.GetAllAuthGroups(ctx); err != nil {
			logging.Errorf(ctx, "Failed to refresh AuthGroups: %s", err)
		}
	}
}

// takeGroupsSnapshot takes a snapshot of all the AuthGroups.
//
// Runs a read-only transaction internally.
func takeGroupsSnapshot(ctx context.Context) (snap *AuthGroupsSnapshot, err error) {
	// This is a potentially slow operation. Capture it in the trace.
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/auth_service/impl/servers/groups/takeGroupsSnapshot")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		snap = &AuthGroupsSnapshot{}

		// Get the AuthReplicationState and all AuthGroups in parallel.
		gr, ctx := errgroup.WithContext(ctx)
		gr.Go(func() (dsErr error) {
			latestState, dsErr := model.GetReplicationState(ctx)
			if dsErr != nil {
				return dsErr
			}
			snap.authDBRev = latestState.AuthDBRev
			return nil
		})
		gr.Go(func() (dsErr error) {
			snap.groups, dsErr = model.GetAllAuthGroups(ctx)
			if dsErr != nil {
				return dsErr
			}
			return nil
		})

		return gr.Wait()
	}, &datastore.TransactionOptions{ReadOnly: true})

	if err != nil {
		return nil, errors.Annotate(err, "failed to take a snapshot of all AuthGroups").Err()
	}

	return snap, nil
}
