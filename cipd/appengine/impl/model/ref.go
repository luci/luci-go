// Copyright 2018 The LUCI Authors.
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

package model

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

// Ref represents a named pointer to some package instance.
//
// ID is a ref name, the parent entity is the corresponding Package.
//
// Compatible with the python version of the backend.
type Ref struct {
	_kind  string                `gae:"$kind,PackageRef"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	Name    string         `gae:"$id"`     // e.g. "latest"
	Package *datastore.Key `gae:"$parent"` // see PackageKey()

	InstanceID string    `gae:"instance_id"` // see common.ObjectRefToInstanceID()
	ModifiedBy string    `gae:"modified_by"` // who moved it the last time
	ModifiedTs time.Time `gae:"modified_ts"` // when it was moved the last time
}

// Proto returns cipd.Ref proto with information from this entity.
func (e *Ref) Proto() *api.Ref {
	return &api.Ref{
		Name:       e.Name,
		Package:    e.Package.StringID(),
		Instance:   common.InstanceIDToObjectRef(e.InstanceID),
		ModifiedBy: e.ModifiedBy,
		ModifiedTs: timestamppb.New(e.ModifiedTs),
	}
}

// SetRef moves or creates a ref.
//
// Assumes inputs are already validated. Launches a transaction inside (and thus
// can't be a part of a transaction itself). Updates 'inst' in-place with the
// most recent instance state.
//
// Returns gRPC-tagged errors:
//
//	NotFound if there's no such instance or package.
//	FailedPrecondition if some processors are still running.
//	Aborted if some processors have failed.
func SetRef(ctx context.Context, ref string, inst *Instance) error {
	return Txn(ctx, "SetRef", func(ctx context.Context) error {
		if err := CheckInstanceReady(ctx, inst); err != nil {
			return err
		}

		events := Events{}

		// Do not touch the ref's ModifiedBy/ModifiedTs if it already points to the
		// requested instance. Need to fetch the ref to check this.
		r := Ref{Name: ref, Package: inst.Package}
		switch err := datastore.Get(ctx, &r); {
		case err == datastore.ErrNoSuchEntity:
			break // need to create the new ref
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("failed to fetch the ref: %w", err))
		case r.InstanceID == inst.InstanceID:
			return nil // already set to the requested instance
		default:
			events.Emit(&api.Event{
				Kind:     api.EventKind_INSTANCE_REF_UNSET,
				Package:  inst.Package.StringID(),
				Instance: r.InstanceID,
				Ref:      ref,
			})
		}

		events.Emit(&api.Event{
			Kind:     api.EventKind_INSTANCE_REF_SET,
			Package:  inst.Package.StringID(),
			Instance: inst.InstanceID,
			Ref:      ref,
		})

		err := datastore.Put(ctx, &Ref{
			Name:       ref,
			Package:    inst.Package,
			InstanceID: inst.InstanceID,
			ModifiedBy: string(auth.CurrentIdentity(ctx)),
			ModifiedTs: clock.Now(ctx).UTC(),
		})
		if err != nil {
			return transient.Tag.Apply(err)
		}
		return events.Flush(ctx)
	})
}

// GetRef fetches the given ref.
//
// Returns gRPC-tagged NotFound error if there's no such ref.
func GetRef(ctx context.Context, pkg, ref string) (*Ref, error) {
	r := &Ref{Name: ref, Package: PackageKey(ctx, pkg)}
	switch err := datastore.Get(ctx, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, grpcutil.NotFoundTag.Apply(errors.New("no such ref"))
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to fetch the ref: %w", err))
	}
	return r, nil
}

// DeleteRef removes the ref if it exists.
//
// Does nothing if there's no such ref or package.
func DeleteRef(ctx context.Context, pkg, ref string) error {
	return Txn(ctx, "DeleteRef", func(ctx context.Context) error {
		r, err := GetRef(ctx, pkg, ref)
		switch {
		case grpcutil.Code(err) == codes.NotFound:
			return nil
		case err != nil:
			return err // transient
		}

		err = datastore.Delete(ctx, &Ref{
			Name:    ref,
			Package: PackageKey(ctx, pkg),
		})
		if err != nil {
			return transient.Tag.Apply(err)
		}

		return EmitEvent(ctx, &api.Event{
			Kind:     api.EventKind_INSTANCE_REF_UNSET,
			Package:  pkg,
			Instance: r.InstanceID,
			Ref:      ref,
		})
	})
}

// ListPackageRefs returns all refs in a package, most recently modified first.
//
// Returns an empty list if there's no such package at all.
func ListPackageRefs(ctx context.Context, pkg string) (out []*Ref, err error) {
	q := datastore.NewQuery("PackageRef").
		Ancestor(PackageKey(ctx, pkg)).
		Order("-modified_ts")
	if err := datastore.GetAll(ctx, q, &out); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("datastore query failed: %w", err))
	}
	return
}

// ListInstanceRefs returns all refs that point to a particular instance, most
// recently modified first.
//
// This is a subset of the output of ListPackageRefs for the corresponding
// package.
//
// Assumes 'inst' is a valid Instance, panics otherwise.
//
// Returns an empty list if there's no such instance at all.
func ListInstanceRefs(ctx context.Context, inst *Instance) (out []*Ref, err error) {
	if inst.Package == nil {
		panic("bad Instance")
	}
	q := datastore.NewQuery("PackageRef").
		Ancestor(inst.Package).
		Eq("instance_id", inst.InstanceID).
		Order("-modified_ts")
	if err := datastore.GetAll(ctx, q, &out); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("datastore query failed: %w", err))
	}
	return
}
