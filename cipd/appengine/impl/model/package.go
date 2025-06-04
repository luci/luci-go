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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

const (
	// Hidden can be used in place of 'true' when working with Package.Hidden flag.
	Hidden = true
	// Visible can be used in place of 'false' when working with Package.Hidden flag.
	Visible = false
)

// Package represents a package as it is stored in the datastore.
//
// It is mostly a marker that the package exists plus some minimal metadata
// about this specific package. Metadata for the package prefix is stored
// separately elsewhere (see 'metadata' package). Package instances, tags and
// refs are stored as child entities (see below).
//
// Root entity. ID is the package name.
//
// Compatible with the python version of the backend.
type Package struct {
	_kind  string                `gae:"$kind,Package"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	Name string `gae:"$id"` // e.g. "a/b/c"

	RegisteredBy string    `gae:"registered_by"` // who registered it
	RegisteredTs time.Time `gae:"registered_ts"` // when it was registered

	Hidden bool `gae:"hidden"` // if true, hide from the listings
}

// PackageKey returns a datastore key of some package, given its name.
func PackageKey(ctx context.Context, pkg string) *datastore.Key {
	return datastore.NewKey(ctx, "Package", pkg, 0, nil)
}

// ListPackages returns a list of names of packages under the given prefix.
//
// Lists all packages recursively. If there's package named as 'prefix' it is
// NOT included in the result. Only packaged under the prefix are included.
//
// The result is sorted by the package name. Returns only transient errors.
func ListPackages(ctx context.Context, prefix string, includeHidden bool) (out []string, err error) {
	if prefix, err = common.ValidatePackagePrefix(prefix); err != nil {
		return nil, err
	}

	// Note: __key__ queries are already ordered by key.
	q := datastore.NewQuery("Package")
	if prefix != "" {
		q = q.Gt("__key__", PackageKey(ctx, prefix+"/ "))
		q = q.Lt("__key__", PackageKey(ctx, prefix+"/~"))
	}

	err = datastore.Run(ctx, q, func(p *Package) error {
		// We filter by Hidden manually since not all entities in the datastore have
		// it set, so filtering using Eq("Hidden", false) actually skips all
		// entities that don't have Hidden field at all.
		if !p.Hidden || includeHidden {
			out = append(out, p.Name)
		}
		return nil
	})
	if err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("failed to query the list of packages: %w", err))
	}
	return out, nil
}

// CheckPackages given a list of package names returns packages that exist, in
// the order they are listed in the list.
//
// If includeHidden is false, omits hidden packages from the result.
//
// Returns only transient errors.
func CheckPackages(ctx context.Context, names []string, includeHidden bool) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	pkgs := make([]*Package, len(names))
	for i, n := range names {
		pkgs[i] = &Package{Name: n}
	}

	if err := datastore.Get(ctx, pkgs); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return nil, transient.Tag.Apply(err)
		}
		existing := pkgs[:0]
		for i, pkg := range pkgs {
			switch err := merr[i]; {
			case err == nil:
				existing = append(existing, pkg)
			case err != datastore.ErrNoSuchEntity:
				return nil, transient.Tag.Apply(errors.Fmt("failed to fetch %q: %w", pkg.Name, err))
			}
		}
		pkgs = existing
	}

	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		if !p.Hidden || includeHidden {
			out = append(out, p.Name)
		}
	}
	return out, nil
}

// CheckPackageExists verifies the package exists.
//
// Returns gRPC-tagged NotFound error if there's no such package.
func CheckPackageExists(ctx context.Context, pkg string) error {
	switch res, err := CheckPackages(ctx, []string{pkg}, true); {
	case err != nil:
		return errors.Fmt("failed to check the package presence: %w", err)
	case len(res) == 0:
		return grpcutil.NotFoundTag.Apply(errors.Fmt("no such package: %s", pkg))
	default:
		return nil
	}
}

// SetPackageHidden updates Hidden field of the package.
//
// If the package is missing returns datastore.ErrNoSuchEntity. All other errors
// are transient.
func SetPackageHidden(ctx context.Context, pkg string, hidden bool) error {
	return Txn(ctx, "SetPackageHidden", func(ctx context.Context) error {
		p := &Package{Name: pkg}
		switch err := datastore.Get(ctx, p); {
		case err == datastore.ErrNoSuchEntity:
			return err
		case err != nil:
			return transient.Tag.Apply(err)
		case p.Hidden == hidden:
			return nil
		}

		p.Hidden = hidden
		if err := datastore.Put(ctx, p); err != nil {
			return transient.Tag.Apply(err)
		}

		ev := api.EventKind_PACKAGE_HIDDEN
		if !hidden {
			ev = api.EventKind_PACKAGE_UNHIDDEN
		}
		return EmitEvent(ctx, &api.Event{Kind: ev, Package: pkg})
	})
}
