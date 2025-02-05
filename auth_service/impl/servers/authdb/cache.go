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

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

// The maximum amount of time since a permissions snapshot was taken before it is
// considered too stale and should be refreshed.
const maxStaleness = 30 * time.Second

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

type PermissionsSnapshot struct {
	authDBRev      int64
	permissionsMap map[string][]*rpcpb.RealmPermissions
}

type CachingPermissionsProvider struct {
	cached breadbox.Breadbox
}

// GetAllPermissions gets realm and analyzes the principal permissions mapping.
// The result may be stale.
func (cgp *CachingPermissionsProvider) GetAllPermissions(ctx context.Context) (map[string][]*rpcpb.RealmPermissions, error) {
	val, err := cgp.cached.Get(ctx, maxStaleness, permissionsRefresher)

	if err != nil {
		return nil, err
	}

	return val.(*PermissionsSnapshot).permissionsMap, nil
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
		return nil, errors.Annotate(err,
			"failed to fetch the latest AuthDBSnapshot revision").Err()
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

	// Transactionally fetch permissions mapping.
	logging.Debugf(ctx, "Fetching permissions from Datastore...")
	var realms *protocol.Realms
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		snap = &PermissionsSnapshot{}

		// Get revision of latest snapshot.
		latestState, dsErr := model.GetAuthDBSnapshotLatest(ctx)
		if dsErr != nil {
			return dsErr
		}
		snap.authDBRev = latestState.AuthDBRev
		// Get realms of latest snapshot.
		realms, dsErr = model.GetRealms(ctx, snap.authDBRev)
		if dsErr != nil {
			return dsErr
		}
		return nil
	}, &datastore.TransactionOptions{ReadOnly: true})
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch latest realms").Err()
	}

	snap.permissionsMap, err = model.AnalyzePrincipalPermissions(realms)
	if err != nil {
		return nil, errors.Annotate(err, "error analyzing permissions").Err()
	}

	logging.Debugf(ctx, "Fetched Permissions (rev %d)", snap.authDBRev)
	return snap, nil
}

// RefreshPeriodically runs a loop that periodically refreshes the cached copy
// of Permissions snapshot.
func (cgp *CachingPermissionsProvider) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, maxStaleness); r.Err != nil {
			return // the context is canceled
		}
		if _, err := cgp.GetAllPermissions(ctx); err != nil {
			logging.Errorf(ctx, "Failed to refresh Permissions: %s", err)
		}
	}
}
