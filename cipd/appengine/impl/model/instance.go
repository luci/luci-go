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
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
)

// Instance represents a package instance as it is stored in the datastore.
//
// Contains some instance metadata, in particular a list of processors that
// scanned the instance (see below).
//
// The parent entity is the corresponding package entity. ID is derived from
// package instance file digest, see ObjectRefToInstanceID().
//
// Compatible with the python version of the backend.
type Instance struct {
	_kind  string                `gae:"$kind,PackageInstance"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	InstanceID string         `gae:"$id"`     // see ObjectRefToInstanceID()
	Package    *datastore.Key `gae:"$parent"` // see PackageKey()

	RegisteredBy string    `gae:"registered_by"` // who registered it
	RegisteredTs time.Time `gae:"registered_ts"` // when it was registered

	// Names of currently running or scheduled processors.
	ProcessorsPending []string `gae:"processors_pending"`
	// Names of processors that successfully finished the processing.
	ProcessorsSuccess []string `gae:"processors_success"`
	// Names of processors that returned fatal errors.
	ProcessorsFailure []string `gae:"processors_failure"`
}

// Proto returns cipd.Instance proto with information from this entity.
func (e *Instance) Proto() *api.Instance {
	return &api.Instance{
		Package:      e.Package.StringID(),
		Instance:     InstanceIDToObjectRef(e.InstanceID),
		RegisteredBy: e.RegisteredBy,
		RegisteredTs: google.NewTimestamp(e.RegisteredTs),
	}
}

// FromProto fills in the entity based on the proto message.
//
// Returns the entity itself for easier chaining.
//
// Doesn't touch output-only fields at all.
func (e *Instance) FromProto(c context.Context, p *api.Instance) *Instance {
	e.InstanceID = ObjectRefToInstanceID(p.Instance)
	e.Package = PackageKey(c, p.Package)
	return e
}

// ObjectRefToInstanceID returns an Instance ID that matches the given CAS
// object ref.
//
// The ref is not checked for correctness. Use cas.ValidateObjectRef if this is
// a concern. Panics if something is not right.
//
// For compatibility with existing data, Instance IDs of packages that use SHA1
// refs are just hex encoded SHA1 digest.
func ObjectRefToInstanceID(ref *api.ObjectRef) string {
	switch ref.HashAlgo {
	case api.HashAlgo_SHA1:
		return ref.HexDigest
	default:
		panic(fmt.Sprintf("unrecognized hash algo %d", ref.HashAlgo))
	}
}

// InstanceIDToObjectRef is a reverse of ObjectRefToInstanceID.
//
// It doesn't check the instance ID for correctness. Panics if it's wrong.
func InstanceIDToObjectRef(iid string) *api.ObjectRef {
	ref := &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA1, // TODO(vadimsh): Recognize more.
		HexDigest: iid,
	}
	if err := cas.ValidateObjectRef(ref); err != nil {
		panic(fmt.Sprintf("bad instance ID %q, the resulting ref is broken - %s", iid, err))
	}
	return ref
}

// RegisterInstance transactionally registers an instance (and the corresponding
// package), if it isn't registered already.
//
// Calls the given callback (inside the transaction) if it is indeed registering
// a new instance. The callback may mutate the instance entity before it is
// stored. The callback may be called multiple times in case of retries (each
// time it will be given a fresh instance to be mutated).
//
// Returns (true, entity, nil) if the instance was registered just now or
// (false, entity, nil) if it existed before.
//
// In either case, it returns the entity that is stored now in the datastore.
// It is either the new instance, or something that existed there before.
func RegisterInstance(c context.Context, inst *Instance, cb func(context.Context, *Instance) error) (reg bool, out *Instance, err error) {
	err = Txn(c, "RegisterInstance", func(c context.Context) error {
		// Reset the state in case of a txn retry.
		reg = false
		out = nil

		// Such instance is already registered?
		existing := Instance{
			InstanceID: inst.InstanceID,
			Package:    inst.Package,
		}
		switch err := datastore.Get(c, &existing); {
		case err == nil:
			out = &existing
			return nil
		case err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to fetch the instance entity").Tag(transient.Tag).Err()
		}

		// Register the package entity too, if missing.
		switch res, err := datastore.Exists(c, inst.Package); {
		case err != nil:
			return errors.Annotate(err, "failed to fetch the package entity existence").Tag(transient.Tag).Err()
		case !res.Any():
			err := datastore.Put(c, &Package{
				Name:         inst.Package.StringID(),
				RegisteredBy: inst.RegisteredBy,
				RegisteredTs: inst.RegisteredTs,
			})
			if err != nil {
				return errors.Annotate(err, "failed to create the package entity").Tag(transient.Tag).Err()
			}
		}

		// Let the caller do more stuff inside this txn, e.g start TQ tasks.
		toPut := *inst
		if cb != nil {
			if err := cb(c, &toPut); err != nil {
				return errors.Annotate(err, "instance registration callback error").Err()
			}
		}

		// Finally register the package instance entity.
		if err := datastore.Put(c, &toPut); err != nil {
			return errors.Annotate(err, "failed to create the package instance entity").Tag(transient.Tag).Err()
		}
		reg = true
		out = &toPut
		return nil
	})
	return
}

// ListInstances lists instances of a package, more recent first.
//
// Only does a query over Instances entities. Doesn't check whether the Package
// entity exists. Returns up to pageSize entities, plus non-nil cursor (if
// there are more results). If pageSize is <= 0, will fetch all entities.
func ListInstances(c context.Context, pkg string, pageSize int32, cursor datastore.Cursor) (out []*Instance, nextCur datastore.Cursor, err error) {
	q := datastore.NewQuery("PackageInstance").Ancestor(PackageKey(c, pkg))
	q = q.Order("-registered_ts")
	if pageSize > 0 {
		q = q.Limit(pageSize)
	}
	if cursor != nil {
		q = q.Start(cursor)
	}
	err = datastore.Run(c, q, func(ent *Instance, cb datastore.CursorCB) error {
		out = append(out, ent)
		if pageSize != 0 && len(out) >= int(pageSize) {
			if nextCur, err = cb(); err != nil {
				return err
			}
			return datastore.Stop
		}
		return nil
	})
	if err != nil {
		return nil, nil, transient.Tag.Apply(err)
	}
	return
}

// CheckInstanceReady fetches the instance and verifies it exists and has zero
// pending or failed processors.
//
// Can be called as part of a transaction. Updates 'inst' in place.
//
// Returns gRPC-tagged errors:
//    NotFound if there's no such instance or package.
//    FailedPrecondition if some processors are still running.
//    Aborted if some processors have failed.
func CheckInstanceReady(c context.Context, inst *Instance) error {
	switch err := datastore.Get(c, inst); {
	case err == datastore.ErrNoSuchEntity:
		// Maybe the package is missing completely?
		switch exists, err := CheckPackage(c, inst.Package.StringID(), true); {
		case err != nil:
			return errors.Annotate(err, "failed to check the package presence").Tag(transient.Tag).Err()
		case !exists:
			return errors.Reason("no such package").Tag(grpcutil.NotFoundTag).Err()
		}
		return errors.Reason("no such instance").Tag(grpcutil.NotFoundTag).Err()
	case err != nil:
		return errors.Annotate(err, "failed to check the instance presence").Tag(transient.Tag).Err()
	case len(inst.ProcessorsFailure) != 0:
		return errors.Reason("some processors failed to process this instance: %s",
			strings.Join(inst.ProcessorsFailure, ", ")).Tag(grpcutil.AbortedTag).Err()
	case len(inst.ProcessorsPending) != 0:
		return errors.Reason("the instance is not ready yet, pending processors: %s",
			strings.Join(inst.ProcessorsPending, ", ")).Tag(grpcutil.FailedPreconditionTag).Err()
	default:
		return nil
	}
}
