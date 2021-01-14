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

// Package lease provides a way to "lock" an external asset with expiration
// time so that concurrent processes/task executions can can achieve
// exclusive modification on that asset.
package lease

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
)

// AssetID is an ID identifying external asset (e.g. Gerrit CL).
//
// It is in the format of "type/value" where 'type' is the type of asset
// and 'value' is the string id which identifies the asset.
type AssetID string

// MakeGCLAssetID returns AssetID of a Gerrit CL.
func MakeGCLAssetID(host string, change int64) AssetID {
	return AssetID(fmt.Sprintf("GerritCL/%s/%d", host, change))
}

func (id AssetID) isValid() bool {
	if i := strings.IndexByte(string(id), '/'); i > 0 && i < len(id)-1 {
		return true
	}
	return false
}

// Lease is like a mutex on external asset with expiration time.
type Lease struct {
	_kind string `gae:"$kind,Lease"`
	// AssetID is the id of the asset that this Lease operates on.
	AssetID AssetID `gae:"$id"`
	// Leasee has the privilege to modify the asset before expiration.
	Leasee string `gae:",noindex"`
	// Action records the action that leasee intends to perform during
	// the lease period.
	Action []byte `gae:",noindex"`
	// ExpirationTime is the time this Lease expires.
	ExpirationTime time.Time `gae:",noindex"`
	// Number is 1-based auto-incremented number upon each successful Lease
	// application.
	Number int64 `gae:",noindex"`
}

// ErrConflict is returned when Asset is currently in lease so that operations
// like `Apply` can't proceed.
var ErrConflict = errors.New("Asset is currently in lease")

// Apply applies for a lease for the given `duration`.
//
// Returns ErrConflict if the asset is already in lease.
func (l *Lease) Apply(ctx context.Context, duration time.Duration) error {
	switch {
	case l == nil:
		return errors.Reason("lease is nil").Err()
	case !l.AssetID.isValid():
		return errors.Reason("invalid AssetID: %q", l.AssetID).Err()
	case l.Leasee == "":
		return errors.Reason("empty leasee").Err()
	case duration < 0:
		return errors.Reason("expected positive lease duration; got %s", duration).Err()
	}
	l.ExpirationTime = datastore.RoundTime(clock.Now(ctx).UTC().Add(duration))
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		curLease := Lease{AssetID: l.AssetID}
		switch err := datastore.Get(ctx, &curLease); {
		case err == datastore.ErrNoSuchEntity: // Asset is leased for the first time
		case err != nil:
			return errors.Annotate(err, "failed to fetch lease for asset %s", curLease.AssetID).Tag(transient.Tag).Err()
		case !curLease.Expired(ctx):
			return ErrConflict
		}
		l.Number = curLease.Number + 1
		if err := datastore.Put(ctx, l); err != nil {
			return errors.Annotate(err, "failed to put new Lease for asset %s", l.AssetID).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)
}

// TODO(yiwzhang): Support Extend() if needed.

// Expired tells whether the Lease has expired or not.
func (l *Lease) Expired(ctx context.Context) bool {
	return clock.Now(ctx).After(l.ExpirationTime)
}

func (l *Lease) validate() error {
	switch {
	case l == nil:
		return errors.Reason("lease is nil").Err()
	case !l.AssetID.isValid():
		return errors.Reason("invalid AssetID: %q", l.AssetID).Err()
	case l.Leasee == "":
		return errors.Reason("empty leasee").Err()
	}
	return nil
}

// Terminate terminates the lease if the supplied lease number is current and
// hasn't expired yet.
func Terminate(ctx context.Context, aid AssetID, leaseNum int64) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		curLease := Lease{AssetID: aid}
		switch err := datastore.Get(ctx, &curLease); {
		case err == datastore.ErrNoSuchEntity:
		case err != nil:
			return errors.Annotate(err, "failed to fetch Lease for asset %s", curLease.AssetID).Tag(transient.Tag).Err()
		case curLease.Number != leaseNum:
		case curLease.Expired(ctx):
		default:
			if err := datastore.Put(ctx, &Lease{AssetID: aid, Number: leaseNum}); err != nil {
				return errors.Annotate(err, "failed to put Lease for asset %s", aid).Tag(transient.Tag).Err()
			}
		}
		return nil
	}, nil)
}
