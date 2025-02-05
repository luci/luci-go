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
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/breadbox"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/impl/model"
)

// The maximum amount of time since a groups snapshot was taken before it is
// considered too stale and should be refreshed.
const maxStaleness = 30 * time.Second

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

type AuthGroupsSnapshot struct {
	authDBRev int64
	groups    []*model.AuthGroup
}

type CachingGroupsProvider struct {
	cached breadbox.Breadbox
}

// GetAllAuthGroups gets all AuthGroups. The result may be slightly stale if
// allowStale is true.
func (cgp *CachingGroupsProvider) GetAllAuthGroups(ctx context.Context, allowStale bool) ([]*model.AuthGroup, error) {
	var val any
	var err error
	if allowStale {
		val, err = cgp.cached.Get(ctx, maxStaleness, groupsRefresher)
	} else {
		val, err = cgp.cached.GetFresh(ctx, groupsRefresher)
	}

	if err != nil {
		return nil, err
	}

	return val.(*AuthGroupsSnapshot).groups, nil
}

func groupsRefresher(ctx context.Context, prev any) (updated any, err error) {
	if prev == nil {
		updated, err = fetch(ctx)
	} else {
		updated, err = refetch(ctx, prev.(*AuthGroupsSnapshot))
	}
	if err != nil {
		updated = nil
	}

	return
}

// refetch fetches all AuthGroups only if outdated; otherwise, returns `prev`.
func refetch(ctx context.Context, prev *AuthGroupsSnapshot) (*AuthGroupsSnapshot, error) {
	// Grab the latest AuthDB revision number.
	latestState, err := model.GetReplicationState(ctx)
	if err != nil {
		return nil, errors.Annotate(err,
			"failed to check the latest AuthDB revision").Err()
	}

	if prev.authDBRev >= latestState.AuthDBRev {
		logging.Debugf(ctx, "Using cached AuthGroups from rev %d", prev.authDBRev)
		return prev, nil
	}

	// Fetch all groups.
	return fetch(ctx)
}

func fetch(ctx context.Context) (snap *AuthGroupsSnapshot, err error) {
	// This is a potentially slow operation. Capture it in the trace.
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/auth_service/impl/servers/groups/fetch")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Transactionally fetch all groups.
	logging.Debugf(ctx, "Fetching AuthGroups from Datastore...")
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
		return nil, errors.Annotate(err, "failed to fetch all AuthGroups").Err()
	}

	logging.Debugf(ctx, "Fetched AuthGroups (rev %d)", snap.authDBRev)
	return snap, nil
}

// RefreshPeriodically runs a loop that periodically refreshes the cached copy
// of AuthGroups snapshot.
func (cgp *CachingGroupsProvider) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, maxStaleness); r.Err != nil {
			return // the context is canceled
		}
		if _, err := cgp.GetAllAuthGroups(ctx, false); err != nil {
			logging.Errorf(ctx, "Failed to refresh AuthGroups: %s", err)
		}
	}
}
