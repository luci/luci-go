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

package cipd

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/sync/promise"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
)

// Resolver resolves versions of packages in an ensure file into concrete
// instance IDs.
//
// For versions that are already defined as instance IDs, it verifies they
// actually exist.
//
// The instance of Resolver is stateful. It caches results of resolutions and
// verifications, so that subsequent attempts to resolve/verify same pins are
// fast.
//
// Resolver can be safely used concurrently.
type Resolver struct {
	// Client is the CIPD client to use for resolving versions.
	Client Client

	// VerifyPresence specifies whether the resolver should check the resolved
	// versions actually exist on the backend.
	VerifyPresence bool

	// Visitor is called for each package version that the resolver has
	// successfully processed.
	//
	// It receives the original (pkg, version) tuple and an instance ID it
	// resolves to.
	//
	// Called concurrently from multiple goroutines in undefined order. Same
	// (pkg, version) tuple may be visited multiple times. May be called for noop
	// version resolutions (when the version is already given as an instance ID).
	Visitor func(pkg, ver, iid string)

	resolving promise.Map[unresolvedPkg, common.Pin]
	verifying promise.Map[common.Pin, struct{}]
}

type unresolvedPkg struct {
	pkg string
	ver string
}

func (p unresolvedPkg) String() string { return fmt.Sprintf("%s@%s", p.pkg, p.ver) }

// ResolvePackage resolves the package's version into a concrete instance ID
// and (if Resolver.VerifyPresence is true) verifies it exists.
func (r *Resolver) ResolvePackage(ctx context.Context, pkg, ver string) (pin common.Pin, err error) {
	if err = common.ValidatePackageName(pkg); err != nil {
		return
	}
	if err = common.ValidateInstanceVersion(ver); err != nil {
		return
	}

	// If a version looks like IID already just use it as a pin right away.
	if common.ValidateInstanceID(ver, common.AnyHash) == nil {
		pin = common.Pin{PackageName: pkg, InstanceID: ver}
	} else {
		pin, err = r.resolveVersion(ctx, pkg, ver)
		if err != nil {
			return
		}
	}

	if r.VerifyPresence {
		err = r.verifyPin(ctx, pin)
	}

	if err == nil && r.Visitor != nil {
		r.Visitor(pkg, ver, pin.InstanceID)
	}

	return
}

// Resolve resolves versions of all packages in the ensure file using the
// given expander to expand templates.
//
// Succeeds only if all packages have been successfully resolved and verified.
//
// Names of packages that failed the resolution are returned as part of the
// multi-error.
func (r *Resolver) Resolve(ctx context.Context, file *ensure.File, expander template.Expander) (*ensure.ResolvedFile, error) {
	return file.Resolve(func(pkg, ver string) (common.Pin, error) {
		return r.ResolvePackage(ctx, pkg, ver)
	}, expander)
}

// ResolveAllPlatforms resolves the ensure file for all platform it is verified
// for (see file.VerifyPlatforms list).
//
// Doesn't stop on a first error. Collects them all into a single multi-error.
func (r *Resolver) ResolveAllPlatforms(ctx context.Context, file *ensure.File) (map[template.Platform]*ensure.ResolvedFile, error) {
	logging.Debugf(ctx, "Resolving for %d platform(s) in the ensure file...", len(file.VerifyPlatforms))

	r.Client.BeginBatch(ctx)
	defer r.Client.EndBatch(ctx)

	type resolvedOrErr struct {
		resolved *ensure.ResolvedFile
		err      error
	}
	results := make([]resolvedOrErr, len(file.VerifyPlatforms))

	// Note: errors are reported through 'results'.
	parallel.FanOutIn(func(tasks chan<- func() error) {
		for idx, plat := range file.VerifyPlatforms {
			idx := idx
			plat := plat
			tasks <- func() error {
				ret, err := r.Resolve(ctx, file, plat.Expander())
				results[idx] = resolvedOrErr{ret, err}
				return nil
			}
		}
	})

	// Collect all errors into a flat MultiError list sorted by platform.
	out := make(map[template.Platform]*ensure.ResolvedFile, len(results))
	var merr errors.MultiError
	for idx, plat := range file.VerifyPlatforms {
		err := results[idx].err
		if err == nil {
			out[plat] = results[idx].resolved
			continue
		}
		if me, ok := err.(errors.MultiError); ok {
			for _, err := range me {
				merr = append(merr, errors.Annotate(err, "when resolving %s", plat).Err())
			}
		} else {
			merr = append(merr, errors.Annotate(err, "when resolving %s", plat).Err())
		}
	}
	if len(merr) != 0 {
		return nil, merr
	}
	return out, nil
}

////////////////////////////////////////////////////////////////////////////////

// resolveVersion returns a resolved pin for the given package or an error.
func (r *Resolver) resolveVersion(ctx context.Context, pkg, ver string) (common.Pin, error) {
	unresolved := unresolvedPkg{pkg, ver}

	promise := r.resolving.Get(ctx, unresolved, func(ctx context.Context) (common.Pin, error) {
		logging.Debugf(ctx, "Resolving package %s ...", unresolved)
		pin, err := r.Client.ResolveVersion(ctx, unresolved.pkg, unresolved.ver)
		if err == nil {
			logging.Debugf(ctx, "Resolved package %s => %s", unresolved, pin)
		} else {
			logging.Debugf(ctx, "Failed to resolve package %s: %s", unresolved, err)
		}
		return pin, err
	})

	pin, err := promise.Get(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	return pin, nil
}

// verifyPin returns nil if the given pin exists on the backend.
func (r *Resolver) verifyPin(ctx context.Context, pin common.Pin) error {
	promise := r.verifying.Get(ctx, pin, func(ctx context.Context) (struct{}, error) {
		logging.Debugf(ctx, "Validating pin %s...", pin)
		_, err := r.Client.DescribeInstance(ctx, pin, nil)
		if err == nil {
			logging.Debugf(ctx, "Pin %s successfully validated", pin)
		} else {
			logging.Debugf(ctx, "Failed to resolve instance info for %s: %s", pin, err)
		}
		return struct{}{}, err
	})
	_, err := promise.Get(ctx)
	return err
}
