// Copyright 2021 The LUCI Authors.
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

package impl

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/authdb"

	"go.chromium.org/luci/auth_service/impl/model"
)

var tracer = otel.Tracer("go.chromium.org/luci/auth_service")

// AuthDBProvider knows how to produce an up-to-date authdb.DB instance.
//
// It caches it in memory, refetching it from Datastore when it detects the
// cached copy is stale.
type AuthDBProvider struct {
	m      sync.RWMutex
	cached *authdb.SnapshotDB
}

// GetAuthDB returns the latest authdb.DB instance to use for ACL checks.
//
// Refetches it from the datastore if necessary.
func (a *AuthDBProvider) GetAuthDB(ctx context.Context) (db authdb.DB, err error) {
	ctx, span := tracer.Start(ctx, "go.chromium.org/luci/auth_service/impl/GetAuthDB")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Grab the latest AuthDB revision number in the datastore.
	latestState, err := model.GetReplicationState(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to check the latest AuthDB revision: %w", err)
	}

	// Use the cached copy if it is up-to-date.
	a.m.RLock()
	cached := a.cached
	a.m.RUnlock()
	if cached != nil && cached.Rev == latestState.AuthDBRev {
		return cached, nil
	}

	if cached == nil {
		logging.Infof(ctx, "Initializing AuthDB")
	} else {
		logging.Infof(ctx, "Refreshing AuthDB (have rev %d, want rev >=%d)", cached.Rev, latestState.AuthDBRev)
	}

	// Fetch the fresh copy from the datastore. Make all other callers of
	// GetAuthDB wait to avoid all of them hitting the datastore at once when
	// the cached copy expires.
	a.m.Lock()
	defer a.m.Unlock()

	// Maybe someone else already fetched a fresher copy while we were waiting
	// on the lock.
	if a.cached != nil && a.cached.Rev >= latestState.AuthDBRev {
		logging.Infof(ctx, "Other goroutine fetched AuthDB rev %d already", a.cached.Rev)
		return a.cached, nil
	}

	logging.Infof(ctx, "Fetching AuthDB from the datastore")

	// Transactionally fetch all data (including AuthReplicationState with the
	// freshest revision) and convert it into an authdb.SnapshotDB.
	snap, err := model.TakeSnapshot(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to make AuthDB snapshot: %w", err)
	}
	snapDB, err := snap.ToAuthDB(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to process AuthDB snapshot: %w", err)
	}

	logging.Infof(ctx, "Fetched AuthDB rev %d", snapDB.Rev)

	a.cached = snapDB
	return snapDB, nil
}

// RefreshPeriodically runs a loop that periodically refreshes the cached copy
// of AuthDB.
func (a *AuthDBProvider) RefreshPeriodically(ctx context.Context) {
	for {
		if r := <-clock.After(ctx, 30*time.Second); r.Err != nil {
			return // the context is canceled
		}
		if _, err := a.GetAuthDB(ctx); err != nil {
			logging.Errorf(ctx, "Failed to refresh AuthDB: %s", err)
		}
	}
}
