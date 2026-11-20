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
	"iter"
	"slices"
	"strings"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
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

// ListPackages returns an iterator over names of packages under the given prefix.
//
// Lists all packages recursively. If there's a package named as 'prefix' it is
// NOT included in the result. Only packages under the prefix are included.
//
// `excludeSubprefixes` must be non-empty strict subprefixes of `prefix`; these
// will be excluded entirely from the result.
//
// The result is sorted by the package name. Yields non-transient validation
// errors for invalid prefixes or `excludeSubprefixes`, and transient errors for
// datastore query failures.
func ListPackages(ctx context.Context, prefix string, includeHidden bool, excludeSubprefixes []string) iter.Seq2[string, error] {
	errout := func(err error) iter.Seq2[string, error] {
		return func(yield func(string, error) bool) { yield("", err) }
	}

	prefix, err := common.ValidatePackagePrefix(prefix)
	if err != nil {
		return errout(err)
	}
	// NOTE: Here and below we treat all non-empty prefixes as ending with a
	// slash. This is important because '-' is allowed in package names, and
	// sorts BEFORE the '/' character. If we don't do this then we have to re-add
	// slashes everywhere below (e.g. when sorting subprefixes and when adding
	// bounds to queries).
	//
	// Without this, you could end up with nonsense like "a/b-c/foo/bar" sorting
	// after "a/b/foo/bar" which would be madness.
	if prefix != "" {
		prefix += "/"
	}

	subprefixes, err := normalizeExcludeSubprefixes(prefix, excludeSubprefixes)
	if err != nil {
		return errout(err)
	}

	excluded := stringset.New(len(subprefixes))
	for _, spfx := range subprefixes {
		excluded.Add(strings.TrimSuffix(spfx, "/"))
	}

	return func(yield func(string, error) bool) {
		for q := range packageRangeQueries(ctx, prefix, subprefixes) {
			for p, qErr := range datastore.RunQuery[*Package](ctx, q).Results {
				if qErr != nil {
					yield("", transient.Tag.Apply(errors.Fmt("failed to query the list of packages: %w", qErr)))
					return
				}

				// Note that because of the '-' vs '/' sorting thing, the datastore
				// query which includes regions (between slashes) may still yield
				// prefixes which are excluded.
				if excluded.Has(p.Name) {
					continue
				}

				// We filter by Hidden manually since not all entities in the datastore
				// have it set, so filtering using Eq("Hidden", false) actually skips
				// all entities that don't have the Hidden field at all.
				//
				// TODO(b/567257598): We could probably dsmapper to fix this in like 10
				// minutes...
				if p.Hidden && !includeHidden {
					continue
				}

				if !yield(p.Name, nil) {
					return
				}
			}
		}
	}
}

// normalizeExcludeSubprefixes validates `excludeSubprefixes` against
// normalized `prefix` ("" or ending with "/") and returns the sorted, disjoint
// subprefixes WITH trailing slashes (for query bounds in
// [packageRangeQueries]).
func normalizeExcludeSubprefixes(prefix string, excludeSubprefixes []string) ([]string, error) {
	if len(excludeSubprefixes) == 0 {
		return nil, nil
	}

	subprefixes := make([]string, 0, len(excludeSubprefixes))
	for _, spfx := range excludeSubprefixes {
		spfx, err := common.ValidatePackagePrefix(spfx)
		if err != nil {
			return nil, err
		}
		if spfx == "" || (prefix != "" && !strings.HasPrefix(spfx, prefix)) {
			return nil, errors.Fmt("excludeSubprefixes: %q does not have %q as a prefix", spfx, prefix)
		}
		subprefixes = append(subprefixes, spfx+"/")
	}
	slices.Sort(subprefixes)

	// Drop duplicates and nested subprefixes (e.g. "a/b/c/" under "a/b/"),
	// which are already covered by their ancestor and would otherwise produce
	// empty query ranges (which are [datastore.ErrNullQuery]).
	out := subprefixes[:0]
	for _, spfx := range subprefixes {
		if len(out) > 0 && strings.HasPrefix(spfx, out[len(out)-1]) {
			continue
		}
		out = append(out, spfx)
	}
	subprefixes = out

	return subprefixes, nil
}

// packageRangeQueries yields datastore queries covering all packages under
// `prefix` while excluding each subtree in `exclusions`.
//
// `prefix` must be "" or end with "/". `exclusions` must be sorted, disjoint,
// strict subprefixes of `prefix`, each ending with "/".
//
// Note that because of ASCII character sort orders, the caller must ignore
// (in code) any yielded values which exactly match the paths in exclusions
// (sans trailing slash).
//
// Consider the following:
//
//	prefix = "c/"
//	exclusions = "c/a-b/", "c/a/"  (note: `a-b/` sorts before `a/`!)
//
// This will generate 3 queries:
//
//   - ("c/ " .. "c/a-b/ ") - Includes "c/a" and "c/a-b" (but does not include
//     any packages under them).
//   - ("c/a-b/~" .. "c/a/ ") - Does not include any `exclusions`.
//   - ("c/a/~" .. "c/~") - Does not include any `exclusions`.
func packageRangeQueries(ctx context.Context, prefix string, exclusions []string) iter.Seq[*datastore.Query] {
	// makeQuery returns a Package query which finds all packages in (from, to).
	makeQuery := func(from, to string) *datastore.Query {
		return datastore.NewQuery("Package").
			Gt("__key__", PackageKey(ctx, from)).
			Lt("__key__", PackageKey(ctx, to))
	}

	// For prefix "a/" and exclusions ["a/bc/", "a/q/a/b/"], yields queries:
	//   * GT("a/ ")       AND LT("a/bc/ ")
	//   * GT("a/bc/~")    AND LT("a/q/a/b/ ")
	//   * GT("a/q/a/b/~") AND LT("a/~")
	return func(yield func(*datastore.Query) bool) {
		// " " sorts before all packages under `prefix`.
		//
		// e.g. "a/b/ " is before "a/b/a..."
		// e.g. " " is before "a..."
		lowKey := prefix + " "
		for _, spfx := range exclusions {
			if !yield(makeQuery(lowKey, spfx+" ")) {
				return
			}
			// "~" sorts after all packages under `spfx`.
			//
			// e.g. "a/b/~" is after "a/b/zzzzzz...."
			lowKey = spfx + "~"
		}

		// Finally, yield a query from whatever our final lowKey was (which could
		// be prefix+" ", if subprefixes was empty), until just after the very
		// last package under prefix.
		//
		// In the degenerate case of empty subprefixes, this gets all packages in
		// (prefix+" ", prefix+"~").
		yield(makeQuery(lowKey, prefix+"~"))
	}
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

		ev := repopb.EventKind_PACKAGE_HIDDEN
		if !hidden {
			ev = repopb.EventKind_PACKAGE_UNHIDDEN
		}
		return EmitEvent(ctx, &repopb.Event{Kind: ev, Package: pkg})
	})
}
