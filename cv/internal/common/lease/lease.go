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
)

// ResourceID is an ID identifying external resource (e.g. a Gerrit CL).
//
// It is in the format of "type/value" where 'type' is the type of the
// resource and 'value' is the string id which identifies the resource.
type ResourceID string

func (id ResourceID) isValid() bool {
	if i := strings.IndexByte(string(id), '/'); i > 0 && i < len(id)-1 {
		return true
	}
	return false
}

// Application contains information to apply for a Lease.
type Application struct {
	// ResourceID is the id of the resource that this Lease will operate on.
	//
	// Required and MUST be valid (See comment of `ResourceID` for format).
	ResourceID ResourceID
	// Holder has the privilege to mutate the resource before Lease expiration.
	//
	// Required.
	Holder string
	// Payload is used to record the mutation that the Lease holder intends to
	// perform during the Lease period.
	Payload []byte
	// ExpireTime is the time that this Lease expires.
	//
	// It will be truncated to millisecond precision in the result Lease.
	//
	// Required, MUST be larger than the current time.
	ExpireTime time.Time
}

func (a *Application) validate(ctx context.Context) error {
	switch now := clock.Now(ctx); {
	case a == nil:
		return errors.Reason("nil lease application").Err()
	case !a.ResourceID.isValid():
		return errors.Reason("invalid ResourceID: %q", a.ResourceID).Err()
	case a.Holder == "":
		return errors.Reason("empty lease Holder").Err()
	case now.After(a.ExpireTime.Truncate(time.Millisecond)):
		return errors.Reason("expect ExpireTime: %s larger than now: %s", a.ExpireTime.Truncate(time.Millisecond), now).Err()
	}
	return nil
}

// AlreadyInLeaseErr is returned when resource is currently in lease.
type AlreadyInLeaseErr struct {
	// ResourceID is the ID of the target resource.
	ResourceID ResourceID
	// ExpireTime is the time the current lease on the target rescourse expires.
	ExpireTime time.Time
	// Holder is the holder of the current lease on the target rescourse.
	Holder string
}

// Error implements `error`.
func (e *AlreadyInLeaseErr) Error() string {
	return fmt.Sprintf("Resource %q is currently leased by %s until %s", e.ResourceID, e.Holder, e.ExpireTime)
}

// IsAlreadyInLeaseErr detects and returns `AlreadyInLeaseErr` in the given err.
func IsAlreadyInLeaseErr(err error) (*AlreadyInLeaseErr, bool) {
	var ret *AlreadyInLeaseErr
	errors.WalkLeaves(err, func(leaf error) bool {
		if e, ok := leaf.(*AlreadyInLeaseErr); ok {
			ret = e
			return false
		}
		return true
	})
	return ret, ret != nil
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
	// ExpireTime is the time (in ms precision) this Lease expires.
	ExpireTime time.Time `gae:",noindex"`
	// Token is randomly generated for each successful lease application and
	// extension.
	//
	// It is used for fast equality check.
	Token []byte `gae:",noindex"`
}

// Expired tells whether the Lease has expired or not.
//
// A nil Lease is always expired.
func (l *Lease) Expired(ctx context.Context) bool {
	if l == nil {
		return true
	}
	return clock.Now(ctx).After(l.ExpireTime)
}

// Extend extends the Lease by additional duration.
//
// Returns AlreadyInLeaseErr if this resource is not in the same lease as
// provided.
// The result expireTime will be truncated to millisecond.
func (l *Lease) Extend(ctx context.Context, addition time.Duration) error {
	switch {
	case addition < 0:
		return errors.Reason("expected positive additional duration; got %s", addition).Err()
	case l.Expired(ctx):
		return errors.New("can't extend an expired lease")
	}

	extended := *l
	extended.ExpireTime = l.ExpireTime.UTC().Add(addition).Truncate(time.Millisecond)
	extended.Token = make([]byte, tokenLen)
	if _, err := mathrand.Read(ctx, extended.Token); err != nil {
		return errors.Annotate(err, "failed to generate token for the extension").Err()
	}

	var innerErr error
	finalErr := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		cur, err := Load(ctx, l.ResourceID)
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to fetch lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		case cur == nil:
			return errors.New("target lease doesn't exist in datastore")
		case !bytes.Equal(cur.Token, l.Token):
			return &AlreadyInLeaseErr{
				ExpireTime: cur.ExpireTime,
				Holder:     cur.Holder,
				ResourceID: cur.ResourceID,
			}
		}
		if err := datastore.Put(ctx, &extended); err != nil {
			return errors.Annotate(err, "failed to put lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)

	switch {
	case innerErr != nil:
		return innerErr
	case finalErr != nil:
		return errors.Annotate(finalErr, "failed to extend lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
	}
	*l = extended
	return nil
}

// Terminate terminates the lease.
//
// Returns AlreadyInLeaseErr if the provided lease doesn't currently hold the
// resource.
func (l *Lease) Terminate(ctx context.Context) error {
	var innerErr error
	finalErr := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		cur, err := Load(ctx, l.ResourceID)
		switch {
		case err != nil:
			return errors.Annotate(err, "failed to fetch lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		case cur == nil:
			return nil // lease is already terminated
		case !bytes.Equal(cur.Token, l.Token):
			return &AlreadyInLeaseErr{
				ExpireTime: cur.ExpireTime,
				Holder:     cur.Holder,
				ResourceID: cur.ResourceID,
			}
		}
		if err := datastore.Delete(ctx, l); err != nil {
			return errors.Annotate(err, "failed to delete lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)

	switch {
	case innerErr != nil:
		return innerErr
	case finalErr != nil:
		return errors.Annotate(finalErr, "failed to terminate lease for resource %s", l.ResourceID).Tag(transient.Tag).Err()
	}
	return nil
}

// Load loads the latest Lease (may already be expired) for given resource.
//
// Returns nil Lease if no Lease can be found for the resource.
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

// TryApply checks if the Lease application will go through given the latest
// Lease on the resource.
//
// Returns non-nil error if the application will fail. Otherwise, returns nil
// error and the new Lease assuming applications succeeds.
//
// MUST be called in a datastore transaction and the latest Lease MUST be
// loaded in the same transaction.
func TryApply(ctx context.Context, latestLease *Lease, app Application) (*Lease, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("must be called in transaction context")
	}
	if err := app.validate(ctx); err != nil {
		return nil, err
	}
	if !latestLease.Expired(ctx) {
		return nil, &AlreadyInLeaseErr{
			ExpireTime: latestLease.ExpireTime,
			Holder:     latestLease.Holder,
			ResourceID: latestLease.ResourceID,
		}
	}
	ret := &Lease{
		ResourceID: app.ResourceID,
		Holder:     app.Holder,
		Payload:    app.Payload,
		ExpireTime: app.ExpireTime.UTC().Truncate(time.Millisecond),
		Token:      make([]byte, tokenLen),
	}
	if _, err := mathrand.Read(ctx, ret.Token); err != nil {
		return nil, err
	}
	return ret, nil
}

// Apply applies for a new lease.
//
// Returns AlreadyInLeaseErr if the lease on this resource hasn't expired yet.
func Apply(ctx context.Context, app Application) (*Lease, error) {
	if err := app.validate(ctx); err != nil {
		return nil, err
	}
	rid := app.ResourceID
	var ret *Lease
	var innerErr error
	finalErr := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		cur, err := Load(ctx, rid)
		if err != nil {
			return errors.Annotate(err, "failed to fetch lease for resource %s", rid).Tag(transient.Tag).Err()
		}
		ret, err = TryApply(ctx, cur, app)
		if err != nil {
			return err
		}
		if err := datastore.Put(ctx, ret); err != nil {
			return errors.Annotate(err, "failed to put Lease for resource %s", rid).Tag(transient.Tag).Err()
		}
		return nil
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case finalErr != nil:
		return nil, errors.Annotate(finalErr, "failed to create lease for resource %s", rid).Tag(transient.Tag).Err()
	}
	return ret, nil
}
