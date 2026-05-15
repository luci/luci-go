// Copyright 2026 The LUCI Authors.
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

package cipd

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/client/cipd/ui"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/cipd/common/cipderr"
)

// PackageVersion describes a possibly-unresolved version of a package, and the
// Service which is supposed to have it.
type PackageVersion struct {
	ServiceURL  string
	PackageName string
	Version     string
}

// PackageVersionSet is a set of [PackageVersion]s.
type PackageVersionSet map[PackageVersion]struct{}

// Add ensures that the given triple is in the set.
func (s PackageVersionSet) Add(service, pkg, version string) {
	s[PackageVersion{service, pkg, version}] = struct{}{}
}

type resolver func(ctx context.Context, service, pkg, version string) (common.Pin, error)
type instanceChecker func(ctx context.Context, service, pkg, version string) error

// resolve will attempt to resolve ALL pins, filling them into `vc`.
//
// If they were all successfully resolved, they are returned in a new
// PackageVersionSet where all `versions` are instance IDs. This resolved map
// will be deduplicated (if multiple tags/refs point to the same instance, they
// will be merged into a single entry).
//
// If `checkInstance` is not nil, it will be called for versions that are
// already instance IDs to verify their existence; Otherwise instance IDs are
// passed through without checking.
func (m PackageVersionSet) resolve(ctx context.Context, vc *internal.VersionCache, resolveFn resolver, checkInstance instanceChecker) (ret PackageVersionSet, err error) {
	now := clock.Now(ctx)

	total := int64(len(m))
	ctx, done := ui.NewActivity(ctx, nil, "resolving")
	defer func() {
		if err != nil {
			logging.Errorf(ctx, "Failed to resolve all versions")
		} else {
			logging.Infof(ctx, "Resolved %d pins in %.1fs", total, clock.Now(ctx).Sub(now).Seconds())
		}
		done()
	}()

	var mu sync.Mutex
	pins := make(PackageVersionSet, len(m))
	var errs []error
	var progress int64

	curAct := ui.CurrentActivity(ctx)
	// report either reports a pin or an error.
	report := func(service string, pin common.Pin, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			pins.Add(service, pin.PackageName, pin.InstanceID)
		} else {
			errs = append(errs, err)
		}
		progress++
		curAct.Progress(ctx, "resolving", "versions", progress, total)
	}

	var eg errgroup.Group
	// Note: there is no serious computation which is happening here; 99% of the
	// time is waiting for small RPCs from the server.
	eg.SetLimit(64)

	for vers := range m {
		pin := common.Pin{PackageName: vers.PackageName}
		if common.ValidateInstanceID(vers.Version, common.AnyHash) == nil {
			pin.InstanceID = vers.Version
			if checkInstance != nil {
				eg.Go(func() error {
					err := checkInstance(ctx, vers.ServiceURL, vers.PackageName, vers.Version)
					report(vers.ServiceURL, pin, err)
					return nil
				})
			} else {
				report(vers.ServiceURL, pin, nil)
			}
			continue
		}

		eg.Go(func() error {
			rslv, err := resolveFn(ctx, vers.ServiceURL, vers.PackageName, vers.Version)
			if err != nil {
				report(vers.ServiceURL, pin, err)
				return nil
			}

			pin.InstanceID = rslv.InstanceID
			vc.AddVersion(ctx, vers.ServiceURL, pin, vers.Version)
			report(vers.ServiceURL, pin, nil)
			return nil
		})
	}
	_ = eg.Wait()

	if len(errs) > 0 {
		for _, err := range errs {
			logging.Errorf(ctx, "%s", err)
		}
		return nil, errors.Join(errs...)
	}

	return pins, nil
}

// fakeHash is a syntactically valid nonsense hash for [OfflineResolve].
var fakeHash string

func init() {
	h := common.MustNewHash(common.DefaultHashAlgo)
	h.Write([]byte("cacheops"))
	fakeHash = common.ObjectRefToInstanceID(common.ObjectRefFromHash(h))
}

// OfflineResolve transforms an ensure.File into an PackageVersionSet for use
// with [Client.CachePrepare].
//
// This does not make any network access, but instead resolves as much as
// possible with `vf` (typically parsed from the ensure.File itself), but
// leaving all other versions unresolved.
//
// `opts.ServiceURL` will be used if `ef.ServiceURL` is unset. One of these two
// must be provided and valid to ensure that the returned PackageVersionSet is
// well-formed.
func OfflineResolve(opts ClientOptions, ef *ensure.File, vf ensure.VersionsFile) (PackageVersionSet, error) {
	var mu sync.Mutex
	ret := PackageVersionSet{}

	platforms := ef.VerifyPlatforms
	if len(platforms) == 0 {
		platforms = []template.Platform{template.DefaultTemplate()}
	}

	if ef.ServiceURL != "" {
		opts.ServiceURL = ef.ServiceURL
	}
	if err := opts.Normalize(); err != nil {
		return nil, cipderr.BadArgument.Apply(errors.Fmt("OfflineResolve: %w", err))
	}
	// serviceURL will now always be <scheme>://<host> with all slashes, etc.
	// normalized away.
	serviceURL := opts.ServiceURL

	for _, plat := range platforms {
		_, err := ef.Resolve(func(pkg, vers string) (common.Pin, error) {
			mu.Lock()
			defer mu.Unlock()

			if pin, err := vf.ResolveVersion(pkg, vers); err == nil {
				ret.Add(serviceURL, pkg, pin.InstanceID)
			} else {
				ret.Add(serviceURL, pkg, vers)
			}

			// We don't care about the resolved pins from file.Resolve - they just
			// need to validate.
			return common.Pin{PackageName: pkg, InstanceID: fakeHash}, nil
			// We allow duplicates because we are just trying to capture all
			// pins in the file, but do not need to be able to install the
			// extracted instances to disk.
		}, &ensure.ResolveOptions{AllowDuplicates: true}, plat.Expander())
		if err != nil {
			return nil, cipderr.Unknown.Apply(errors.Fmt("resolving platform %q: %w", plat, err))
		}
	}

	return ret, nil
}
