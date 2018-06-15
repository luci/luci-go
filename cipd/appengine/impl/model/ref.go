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
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
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

	InstanceID string    `gae:"instance_id"` // see ObjectRefToInstanceID()
	ModifiedBy string    `gae:"modified_by"` // who moved it the last time
	ModifiedTs time.Time `gae:"modified_ts"` // when it was moved the last time
}

// Proto returns cipd.Ref proto with information from this entity.
func (e *Ref) Proto() *api.Ref {
	return &api.Ref{
		Name:       e.Name,
		Package:    e.Package.StringID(),
		Instance:   InstanceIDToObjectRef(e.InstanceID),
		ModifiedBy: e.ModifiedBy,
		ModifiedTs: google.NewTimestamp(e.ModifiedTs),
	}
}

// SetRef moves or creates a ref.
//
// Assumes inputs are already validated. Launches a transaction inside (and thus
// can't be a part of a transaction itself). Updates 'inst' in-place with the
// most recent instance state.
//
// Returns gRPC-tagged errors:
//    NotFound if there's no such instance or package.
//    FailedPrecondition if some processors are still running.
//    Aborted if some processors have failed.
func SetRef(c context.Context, ref string, inst *Instance, who identity.Identity) error {
	return Txn(c, "SetRef", func(c context.Context) error {
		if err := CheckInstanceReady(c, inst); err != nil {
			return err
		}

		// Do not touch the ref's ModifiedBy/ModifiedTs if it already points to the
		// requested instance. Need to fetch the ref to check this.
		r := Ref{Name: ref, Package: inst.Package}
		switch err := datastore.Get(c, &r); {
		case err == datastore.ErrNoSuchEntity:
			break // need to create the new ref
		case err != nil:
			return errors.Annotate(err, "failed to fetch the ref").Tag(transient.Tag).Err()
		case r.InstanceID == inst.InstanceID:
			return nil // already set to the requested instance
		}

		return transient.Tag.Apply(datastore.Put(c, &Ref{
			Name:       ref,
			Package:    inst.Package,
			InstanceID: inst.InstanceID,
			ModifiedBy: string(who),
			ModifiedTs: clock.Now(c).UTC(),
		}))
	})
}

// GetRef fetches the given ref.
//
// Returns gRPC-tagged NotFound error if there's no such ref.
func GetRef(c context.Context, pkg, ref string) (*Ref, error) {
	r := &Ref{Name: ref, Package: PackageKey(c, pkg)}
	switch err := datastore.Get(c, r); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Reason("no such ref").Tag(grpcutil.NotFoundTag).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch the ref").Tag(transient.Tag).Err()
	}
	return r, nil
}

// DeleteRef removes the ref if it exists.
//
// Does nothing if there's no such ref or package.
func DeleteRef(c context.Context, pkg, ref string) error {
	return transient.Tag.Apply(datastore.Delete(c, &Ref{
		Name:    ref,
		Package: PackageKey(c, pkg),
	}))
}

// ListPackageRefs returns all refs in a package, most recently modified first.
//
// Returns an empty list if there's no such package at all.
func ListPackageRefs(c context.Context, pkg string) (out []*Ref, err error) {
	q := datastore.NewQuery("PackageRef").
		Ancestor(PackageKey(c, pkg)).
		Order("-modified_ts")
	if err := datastore.GetAll(c, q, &out); err != nil {
		return nil, errors.Annotate(err, "datastore query failed").Tag(transient.Tag).Err()
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
func ListInstanceRefs(c context.Context, inst *Instance) (out []*Ref, err error) {
	if inst.Package == nil {
		panic("bad Instance")
	}
	q := datastore.NewQuery("PackageRef").
		Ancestor(inst.Package).
		Eq("instance_id", inst.InstanceID).
		Order("-modified_ts")
	if err := datastore.GetAll(c, q, &out); err != nil {
		return nil, errors.Annotate(err, "datastore query failed").Tag(transient.Tag).Err()
	}
	return
}
