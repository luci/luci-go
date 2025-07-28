// Copyright 2025 The LUCI Authors.
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

package authdb

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/breadbox"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	customerrors "go.chromium.org/luci/auth_service/impl/errors"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/model/graph"
)

// The maximum amount of time since a permissions snapshot was taken before it is
// considered too stale and should be refreshed.
const maxStaleness = 30 * time.Second

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

type PermissionsSnapshot struct {
	authDBRev       int64
	permissionNames []string
	permissionsMap  map[string][]*model.RealmPermissions
	groupsGraph     *graph.Graph
}

type CachingPermissionsProvider struct {
	cached breadbox.Breadbox
}

// Get retrieves the PermissionsSnapshot from the cache (result may be stale).
func (cgp *CachingPermissionsProvider) Get(ctx context.Context) (*PermissionsSnapshot, error) {
	val, err := cgp.cached.Get(ctx, maxStaleness, permissionsRefresher)

	if err != nil {
		return nil, err
	}

	return val.(*PermissionsSnapshot), nil
}

func permissionsRefresher(ctx context.Context, prev any) (updated any, err error) {
	if prev == nil {
		updated, err = fetch(ctx)
	} else {
		updated, err = refetch(ctx, prev.(*PermissionsSnapshot))
	}
	if err != nil {
		updated = nil
	}
	return
}

// refetch fetches all permissions from latest authDB snapshot.
func refetch(ctx context.Context, prev *PermissionsSnapshot) (*PermissionsSnapshot, error) {
	// Grab the latest AuthDB revision number.
	latestState, err := model.GetAuthDBSnapshotLatest(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to fetch the latest AuthDBSnapshot revision: %w", err)
	}

	if latestState.AuthDBRev <= prev.authDBRev {
		logging.Debugf(ctx, "Using cached Permissions from rev %d", prev.authDBRev)
		return prev, nil
	}

	// Fetch all permissions.
	return fetch(ctx)
}

func fetch(ctx context.Context) (snap *PermissionsSnapshot, err error) {
	// This is a potentially slow operation. Capture it in the trace.
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/auth_service/impl/servers/authdb/fetch")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Transactionally fetch the AuthDB from the latest AuthDBSnapshot.
	logging.Debugf(ctx, "Fetching latest AuthDB from Datastore...")
	snap = &PermissionsSnapshot{}
	var authDB *protocol.AuthDB
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get the revision of the latest snapshot.
		latestState, dsErr := model.GetAuthDBSnapshotLatest(ctx)
		if dsErr != nil {
			return dsErr
		}
		snap.authDBRev = latestState.AuthDBRev

		// Get AuthDB of latest snapshot.
		authDB, dsErr = model.GetAuthDBFromSnapshot(ctx, snap.authDBRev)
		if dsErr != nil {
			return dsErr
		}

		return nil
	}, &datastore.TransactionOptions{ReadOnly: true})
	if err != nil {
		return nil, errors.Fmt("failed to fetch latest AuthDB: %w", err)
	}

	realms := authDB.GetRealms()
	if realms == nil {
		return nil, customerrors.ErrAuthDBMissingRealms
	}

	// Get all permission names from the realms.
	permissions := realms.GetPermissions()
	snap.permissionNames = make([]string, len(permissions))
	for i, p := range permissions {
		snap.permissionNames[i] = p.GetName()
	}

	// Create permissions map from the realms.
	snap.permissionsMap, err = model.AnalyzePrincipalPermissions(authDB.Realms)
	if err != nil {
		return nil, errors.Fmt("error analyzing permissions: %w", err)
	}

	// Create a graph from the groups.
	groups := make([]model.GraphableGroup, len(authDB.Groups))
	for i, group := range authDB.Groups {
		groups[i] = model.GraphableGroup(group)
	}
	snap.groupsGraph = graph.NewGraph(groups)

	logging.Debugf(ctx, "Fetched permissions snapshot (rev %d)", snap.authDBRev)
	return snap, nil
}

// RefreshPeriodically runs a loop that periodically refreshes the cached copy
// of Permissions snapshot.
func (cgp *CachingPermissionsProvider) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, maxStaleness); r.Err != nil {
			return // the context is canceled
		}
		if _, err := cgp.Get(ctx); err != nil {
			logging.Errorf(ctx, "Failed to refresh permissions snapshot: %s", err)
		}
	}
}
