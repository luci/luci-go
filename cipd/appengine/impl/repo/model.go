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

package repo

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/cas"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
)

////////////////////////////////////////////////////////////////////////////////
// Core entities.

// packageEntity represents a package as it is stored in the datastore.
//
// It is mostly a marker that the package exists plus some minimal metadata
// about this specific package. Metadata for the package prefix is stored
// separately elsewhere (see 'metadata' package). Package instances, tags and
// refs are stored as child entities (see below).
//
// Root entity. ID is the package name. Compatible with the python version of
// the backend.
type packageEntity struct {
	_kind  string                `gae:"$kind,Package"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	PackageName string `gae:"$id"` // e.g. "a/b/c"

	RegisteredBy string    `gae:"registered_by"` // who registered it
	RegisteredTS time.Time `gae:"registered_ts"` // when it was registered

	Hidden bool `gae:"hidden"` // if true, hide from in listings
}

// packageKey returns a datastore key of some package, given its name.
func packageKey(c context.Context, pkg string) *datastore.Key {
	return datastore.NewKey(c, "Package", pkg, 0, nil)
}

// instanceEntity represents a package instance as it is stored in the
// datastore.
//
// Contains some instance metadata, in particular a list of processors that
// scanned with instance (see below).
//
// Parent of the corresponding Package entity. ID is derived from package
// file digest, see objectRefToIID(). Compatible with the python version of the
// backend.
type instanceEntity struct {
	_kind  string                `gae:"$kind,PackageInstance"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	InstanceID string         `gae:"$id"`     // "abc...", see objectRefToIID()
	Package    *datastore.Key `gae:"$parent"` // see packageKey()

	RegisteredBy string    `gae:"registered_by"` // who registered it
	RegisteredTS time.Time `gae:"registered_ts"` // when it was registered

	// Names of currently running or scheduled processors.
	ProcessorsPending []string `gae:"processors_pending"`
	// Names of processors that successfully finished the processing.
	ProcessorsSuccess []string `gae:"processors_success"`
	// Names of processors that returned fatal errors.
	ProcessorsFailure []string `gae:"processors_failure"`
}

// proto returns Instance proto with information from this entity.
//
// It is what's returned by public APIs.
func (e *instanceEntity) proto() *api.Instance {
	return &api.Instance{
		Package:      e.Package.StringID(),
		Instance:     instanceIDToObjectRef(e.InstanceID),
		RegisteredBy: e.RegisteredBy,
		RegisteredTs: google.NewTimestamp(e.RegisteredTS),
	}
}

// objectRefToInstanceID returns an instanceEntity ID that matches the given
// CAS object ref.
//
// The ref is not checked for correctness. Use cas.ValidateObjectRef if this is
// a concern. Panics if something is not right.
//
// For compatibility with existing data, instanceEntity IDs of packages that use
// SHA1 refs are just hex encoded SHA1 digest.
func objectRefToInstanceID(ref *api.ObjectRef) string {
	switch ref.HashAlgo {
	case api.HashAlgo_SHA1:
		return ref.HexDigest
	default:
		panic(fmt.Sprintf("unrecognized hash algo %d", ref.HashAlgo))
	}
}

// instanceIDToObjectRef is a reverse of objectRefToInstanceID.
//
// It doesn't check the instance ID for correctness. Panics if it's wrong.
func instanceIDToObjectRef(iid string) *api.ObjectRef {
	ref := &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA1, // TODO(vadimsh): Recognize more.
		HexDigest: iid,
	}
	if err := cas.ValidateObjectRef(ref); err != nil {
		panic(fmt.Sprintf("bad instance ID %q, the resulting ref is broken - %s", iid, err))
	}
	return ref
}

// registerInstance transactionally registers an instance (and the corresponding
// package), if it isn't registered already.
//
// Returns (true, ..., ...) if the instance was registered just now or false if
// it existed before. In either case the entity that ends up in the datastore.
func registerInstance(c context.Context, inst *instanceEntity) (reg bool, out *instanceEntity, err error) {
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		// Reset the state in case of a txn retry.
		reg = false
		out = nil

		// Such instance is already registered?
		existing := instanceEntity{
			InstanceID: inst.InstanceID,
			Package:    inst.Package,
		}
		switch err := datastore.Get(c, &existing); {
		case err == nil:
			out = &existing
			return nil
		case err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to fetch the instance entity").Err()
		}

		// Register the package entity too, if missing.
		switch res, err := datastore.Exists(c, inst.Package); {
		case err != nil:
			return errors.Annotate(err, "failed to fetch the package entity existence").Err()
		case !res.Any():
			err := datastore.Put(c, &packageEntity{
				PackageName:  inst.Package.StringID(),
				RegisteredBy: inst.RegisteredBy,
				RegisteredTS: inst.RegisteredTS,
			})
			if err != nil {
				return errors.Annotate(err, "failed to create the package entity").Err()
			}
		}

		// TODO(vadimsh): Add processors support.

		// Finally register the package instance entity.
		if err := datastore.Put(c, inst); err != nil {
			return errors.Annotate(err, "failed to create the package instance entity").Err()
		}
		reg = true
		out = inst
		return nil
	}, nil)
	return
}

////////////////////////////////////////////////////////////////////////////////
// Refs support.

// TODO

////////////////////////////////////////////////////////////////////////////////
// Tags support.

// TODO

////////////////////////////////////////////////////////////////////////////////
// Package processing.

// TODO
