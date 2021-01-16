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

// Package lease provides a way to "lock" an external resource with expiration
// time so that concurrent processes/task executions can achieve exclusive
// privilege to make mutations (generally long-running and non-idempotent)
// on that resource.
package lease

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// ResourceID is an ID identifying external resource (e.g. a Gerrit CL).
//
// It is in the format of "type/value" where 'type' is the type of the
// resource and 'value' is the string id which identifies the resource.
type ResourceID string

// MakeCLResourceID returns ResourceID of a CL in CV.
func MakeCLResourceID(clid common.CLID) ResourceID {
	return ResourceID(fmt.Sprintf("CL/%d", clid))
}

func (id ResourceID) isValid() bool {
	if i := strings.IndexByte(string(id), '/'); i > 0 && i < len(id)-1 {
		return true
	}
	return false
}

// Application contains information to apply for a lease.
type Application struct {
	// ResourceID is the id of the resource that this lease will operate on.
	//
	// Required and MUST be valid (See comment of `ResourceID` for format).
	ResourceID ResourceID
	// Holder has the privilege to mutate the resource before lease expiration.
	//
	// Required.
	Holder string
	// Payload is used to record the mutation that the lease holder intends to
	// perform during the lease period.
	Payload []byte
	// ExpireTime is the time this Lease expires.
	//
	// Required, MUST larger than the current timestamp.
	ExpireTime time.Time
}

func (a *Application) validate(ctx context.Context) error {
	switch {
	case a == nil:
		return errors.Reason("nil lease application").Err()
	case !a.ResourceID.isValid():
		return errors.Reason("invalid ResourceID: %q", a.ResourceID).Err()
	case a.Holder == "":
		return errors.Reason("empty lease Holder").Err()
	case clock.Now(ctx).After(a.ExpireTime):
		return errors.Reason("expect ExpireTime: %s larger than now: %s", a.ExpireTime, clock.Now(ctx)).Err()
	}
	return nil
}

const tokenLen = 8

// Lease is like a mutex on external resource with expiration time.
type Lease struct {
	_kind string `gae:"$kind,Lease"`
	// ResourceID is the id of the resource that this lease will operate on.
	ResourceID ResourceID `gae:"$id"`
	// Holder has the privilege to mutate the resource before lease expiration.
	Holder string `gae:",noindex"`
	// Payload is used to record the mutation that the lease holder intends to
	// perform during the lease period.
	Payload []byte `gae:",noindex"`
	// ExpireTime is the time this Lease expires.
	ExpireTime time.Time `gae:",noindex"`
	// Token is randomly generated for each successful lease application and
	// extension.
	//
	// It is used for fast equality check.
	Token []byte `gae:",noindex"`
}

// Expired tells whether the Lease has expired or not.
//
// A nil lease is always expired
func (l *Lease) Expired(ctx context.Context) bool {
	if l == nil {
		return true
	}
	return clock.Now(ctx).After(l.ExpireTime)
}

// Extend extends the lease by additional duration.
//
// Returns ErrConflict if the lease is not current for the resource.
func (l *Lease) Extend(ctx context.Context, addition time.Duration) error {
	switch {
	case addition < 0:
		return errors.Reason("expect positive additional duration; got %s", addition).Err()
	case l.Expired(ctx):
		return errors.Reason("can't extend an expired lease").Err()
	}
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur, err := Load(ctx, l.ResourceID)
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to fetch lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		case cur == nil:
			return errors.Reason("target lease doesn't exist in datastore").Err()
		case !bytes.Equal(cur.Token, l.Token):
			return ErrConflict
		}
		l.ExpireTime = datastore.RoundTime(l.ExpireTime.UTC().Add(addition))
		if _, err = mathrand.Read(ctx, l.Token); err != nil {
			return err
		}
		if err := datastore.Put(ctx, l); err != nil {
			return errors.Annotate(err, "failed to put Lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)
}

// Terminate terminates the lease.
//
// Returns ErrConflict if the lease is not current for the resource.
func (l *Lease) Terminate(ctx context.Context) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur, err := Load(ctx, l.ResourceID)
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to fetch lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		case !bytes.Equal(cur.Token, l.Token):
			return ErrConflict
		}
		if err := datastore.Delete(ctx, l); err != nil {
			return errors.Annotate(err, "failed to delete Lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)
}

// ErrConflict is returned when resource is currently in lease so that
// operations like `Apply`, `Extend` can't proceed.
var ErrConflict = errors.New("Resource is currently in lease")

// Load loads the latest lease (may already expired) for given resource.
//
// Returns nil lease for no lease can be found for the resource.
func Load(ctx context.Context, rid ResourceID) (*Lease, error) {
	ret := &Lease{ResourceID: rid}
	switch err := datastore.Get(ctx, ret); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return ret, nil
	}
}

// TryApply checks if the lease application will go through given the latest
// lease on the resource.
//
// Returns non-nil error if the application will fails. Otherwise, returns nil
// error and the new lease assuming applications succeeds.
//
// MUST be called in a datastore transaction and the latest lease MUST be
// loaded in the same transaction.
func TryApply(ctx context.Context, latestLease *Lease, app Application) (*Lease, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in transaction context")
	}
	if err := app.validate(ctx); err != nil {
		return nil, err
	}
	if !latestLease.Expired(ctx) {
		return nil, ErrConflict
	}
	ret := &Lease{
		ResourceID: app.ResourceID,
		Holder:     app.Holder,
		Payload:    app.Payload,
		ExpireTime: datastore.RoundTime(app.ExpireTime.UTC()),
		Token:      make([]byte, tokenLen),
	}
	if _, err := mathrand.Read(ctx, ret.Token); err != nil {
		return nil, err
	}
	return ret, nil
}

// Apply applies for a new lease.
//
// Returns ErrConflict if the resource is already in lease.
func Apply(ctx context.Context, app Application) (l *Lease, err error) {
	err = app.validate(ctx)
	if err != nil {
		return
	}
	rid := app.ResourceID
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur, err := Load(ctx, rid)
		if err != nil {
			return errors.Annotate(err, "failed to fetch lease for resource %s", rid).Tag(transient.Tag).Err()
		}
		l, err = TryApply(ctx, cur, app)
		if err != nil {
			return err
		}
		if err := datastore.Put(ctx, l); err != nil {
			l = nil
			return errors.Annotate(err, "failed to put Lease for resource %s", rid).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)
	return
}
