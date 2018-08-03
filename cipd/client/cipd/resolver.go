// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"sort"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

	resolving promise.Map // unresolvedPkg => (common.Pin, error)
	verifying promise.Map // common.Pin => (nil, error)
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
	return
}

// ResolveEnsureFile resolves versions of all packages in the ensure file.
//
// Succeeds only if all packages have been successfully resolved and verified.
// Names of packages that failed the resolution are logged and also returned
// as part of the multi-error.
func (r *Resolver) ResolveEnsureFile(ctx context.Context, file *ensure.File, expander template.Expander) (*ensure.ResolvedFile, error) {
	// Even on a resolution failure, we don't return an error so the resolver
	// continues to try the remaining packages. However, we'll track the error
	// and return it externally.
	type fail struct {
		def ensure.PackageDef
		err error
	}
	var fails []fail
	var mu sync.Mutex

	ret, err := file.Resolve(func(pkg, ver string, def ensure.PackageDef) (common.Pin, error) {
		pin, err := r.ResolvePackage(ctx, pkg, ver)
		if err != nil {
			mu.Lock()
			fails = append(fails, fail{
				def: def,
				err: errors.Annotate(err, "failed to resolve %s@%s (line %d)", pkg, ver, def.LineNo).Err(),
			})
			mu.Unlock()
			return common.Pin{PackageName: pkg}, nil
		}
		return pin, nil
	}, expander)

	if err != nil {
		return nil, err
	}

	// For determinism, we sort the returned errors by the definition line number.
	if len(fails) > 0 {
		sort.Slice(fails, func(i, j int) bool {
			return fails[i].def.LineNo < fails[j].def.LineNo
		})
		merr := make(errors.MultiError, len(fails))
		for i, fail := range fails {
			merr[i] = fail.err
		}
		return nil, merr
	}

	return ret, nil
}

// resolveVersion returns a resolved pin for the given package or an error.
func (r *Resolver) resolveVersion(ctx context.Context, pkg, ver string) (common.Pin, error) {
	unresolved := unresolvedPkg{pkg, ver}

	promise := r.resolving.Get(ctx, unresolved, func(ctx context.Context) (interface{}, error) {
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
	return pin.(common.Pin), nil
}

// verifyPin returns nil if the given pin exists on the backend.
func (r *Resolver) verifyPin(ctx context.Context, pin common.Pin) error {
	promise := r.verifying.Get(ctx, pin, func(ctx context.Context) (interface{}, error) {
		logging.Debugf(ctx, "Validating pin %s...", pin)
		_, err := r.Client.DescribeInstance(ctx, pin, nil)
		if err == nil {
			logging.Debugf(ctx, "Pin %s successfully validated")
		} else {
			logging.Debugf(ctx, "Failed to resolve instance info for %s: %s", pin, err)
		}
		return nil, err
	})
	_, err := promise.Get(ctx)
	return err
}
