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

	"go.chromium.org/luci/cipd/client/cipd/common"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
)

// Verifier uses a Client to verify the validity of packages in parallel.
type Verifier struct {
	// Client is the CIPD client to use for verification.
	Client Client

	initOnce sync.Once

	resolveMu sync.Mutex
	resolveWG sync.WaitGroup

	// pkgPromises maps UnresolvedPackages to Promise[common.Pin].
	pkgPromises map[UnresolvedPackage]*promise.Promise
	// pinPromises maps common.Pin to a resoluton Promise. The Promise does not
	// have a return value, but may return an error if resolution fails. This is
	// used to deduplicate common.Pin resolution.
	pinPromises map[common.Pin]*promise.Promise

	resultMu    sync.Mutex
	result      VerifyResult
	badPins     map[common.Pin]struct{}
	badPackages map[UnresolvedPackage]struct{}
}

// UnresolvedPackage is an unresolved package.
type UnresolvedPackage struct {
	Name    string
	Version string
}

func (p UnresolvedPackage) String() string {
	return fmt.Sprintf("%s@%s", p.Name, p.Version)
}

// Less returns true if this package's (name, version) tuple is alphabetically
// smaller than other's.
func (p *UnresolvedPackage) Less(other UnresolvedPackage) bool { return p.String() < other.String() }

type verifyPinRequest struct {
	ctx context.Context
	pin common.Pin
}

// VerifyResult is the result of a verification.
type VerifyResult struct {
	NumPackages int
	NumPins     int

	// InvalidPackages is the list of invalid package definitions that were
	// encountered.
	InvalidPackages []UnresolvedPackage

	// InvalidPins is a slice of invalid pins.
	InvalidPins []common.Pin
}

// HasErrors returns true if this verification result had any errors.
func (vr *VerifyResult) HasErrors() bool {
	return len(vr.InvalidPackages) > 0 || len(vr.InvalidPins) > 0
}

// ResolvePackage attempts to resolve pkg. It will block until the package is
// resolved, and enqueue its resolved instance into the verification queue.
func (v *Verifier) ResolvePackage(ctx context.Context, pkg UnresolvedPackage) (common.Pin, error) {
	v.initialize()

	// Get or create our resolution Promise.
	//
	// Note that the Promise body executes in a separate goroutine, so the lock
	// isn't held for too long.
	v.resolveMu.Lock()
	p := v.pkgPromises[pkg]
	if p == nil {
		v.resolveWG.Add(1)
		p = promise.New(ctx, func(ctx context.Context) (interface{}, error) {
			defer v.resolveWG.Done()

			logging.Debugf(ctx, "Resolving package %s ...", pkg)
			pin, err := v.Client.ResolveVersion(ctx, pkg.Name, pkg.Version)
			if err != nil {
				logging.Errorf(ctx, "Failed to resolve package %s: %s", pkg, err)
				v.recordPackageResult(pkg, false)
				return nil, err
			}

			logging.Debugf(ctx, "Resolved package %s => %s", pkg, pin)
			v.recordPackageResult(pkg, true)
			v.VerifyPin(ctx, pin)
			return pin, nil
		})
		v.pkgPromises[pkg] = p
	}
	v.resolveMu.Unlock()

	pinIface, err := p.Get(ctx)
	if err != nil {
		return common.Pin{}, err
	}
	return pinIface.(common.Pin), nil
}

// VerifyEnsureFile adds all of the packages in file into the queue, resolving
// them against expander.
//
// VerifyEnsureFile will block until all packages from within the ensure file
// have been evaluated.
//
// If a package fails to resolve, or if ResolveWith reutrns an error,
// VerifyEnsureFile will return an error.
func (v *Verifier) VerifyEnsureFile(ctx context.Context, file *ensure.File, expander template.Expander) error {
	v.initialize()

	// Resolve all instances.
	//
	// Even on resolution failure, we don't return an error so the resolver
	// continues to try the remaining packages. However, we'll track the error
	// and return it externally.
	var resolveErrors errors.MultiError
	_, err := file.ResolveWith(func(pkgName, vers string) (common.Pin, error) {
		pkg := UnresolvedPackage{pkgName, vers}

		pin, err := v.ResolvePackage(ctx, pkg)
		if err != nil {
			resolveErrors = append(resolveErrors, err)
		}
		return pin, nil
	}, expander)

	switch {
	case err != nil:
		return err
	case len(resolveErrors) > 0:
		return resolveErrors
	default:
		return nil
	}
}

// VerifyPin verifies the validity of pin.
func (v *Verifier) VerifyPin(ctx context.Context, pin common.Pin) {
	v.initialize()

	v.resolveMu.Lock()
	p := v.pinPromises[pin]
	if p == nil {
		v.resolveWG.Add(1)
		p = promise.New(ctx, func(ctx context.Context) (interface{}, error) {
			defer v.resolveWG.Done()

			logging.Debugf(ctx, "Validating pin instance: %s", pin)
			_, err := v.Client.FetchInstanceInfo(ctx, pin)
			if err != nil {
				logging.Errorf(ctx, "Failed to resolve instance info for %s@%s: %s",
					pin.PackageName, pin.InstanceID, err)
			}
			v.recordPinResult(pin, err == nil)
			return nil, err
		})
	}
	v.resolveMu.Unlock()

	// Wait for the pin to verify.
	_, _ = p.Get(ctx)
}

// Result blocks pending completion of all outstanding verification, then
// returns a result structure.
//
// Wait may be called at most once. After Wait is called, no new verifications
// may be performed.
func (v *Verifier) Result() *VerifyResult {
	v.initialize()

	// Wait for any outstanding package resolutions to complete.
	v.resolveWG.Wait()

	// Complete our result and return.
	v.completeResult()
	return &v.result
}

func (v *Verifier) initialize() {
	v.initOnce.Do(func() {
		v.pkgPromises = make(map[UnresolvedPackage]*promise.Promise)
		v.pinPromises = make(map[common.Pin]*promise.Promise)

		// Initialize our intermediate result tracking.
		v.badPackages = make(map[UnresolvedPackage]struct{})
		v.badPins = make(map[common.Pin]struct{})
	})
}

func (v *Verifier) recordPackageResult(pkg UnresolvedPackage, valid bool) {
	v.resultMu.Lock()
	defer v.resultMu.Unlock()

	v.result.NumPackages++
	if !valid {
		v.badPackages[pkg] = struct{}{}
	}
}

func (v *Verifier) recordPinResult(pin common.Pin, valid bool) {
	v.resultMu.Lock()
	defer v.resultMu.Unlock()

	v.result.NumPins++
	if !valid {
		v.badPins[pin] = struct{}{}
	}
}

func (v *Verifier) completeResult() {
	res := &v.result
	if len(v.badPackages) > 0 {
		res.InvalidPackages = make([]UnresolvedPackage, 0, len(v.badPackages))
		for pkg := range v.badPackages {
			res.InvalidPackages = append(res.InvalidPackages, pkg)
		}
		sort.Slice(res.InvalidPackages, func(i, j int) bool {
			return res.InvalidPackages[i].Less(res.InvalidPackages[j])
		})
	}

	if len(v.badPins) > 0 {
		res.InvalidPins = make([]common.Pin, 0, len(v.badPins))
		for pin := range v.badPins {
			res.InvalidPins = append(res.InvalidPins, pin)
		}
		sort.Slice(res.InvalidPins, func(i, j int) bool {
			pinI, pinJ := res.InvalidPins[i], res.InvalidPins[j]
			if pinI.PackageName < pinJ.PackageName {
				return true
			}
			return pinI.InstanceID < pinJ.InstanceID
		})
	}
}
