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
	"golang.org/x/net/context"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/common"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
)

// TemplateFunc builds a set of template parameters to augment the default CIPD
// parameter set with.
type TemplateFunc func(context.Context, []*vpython.PEP425Tag) (map[string]string, error)

// PackageLoader is an implementation of venv.PackageLoader that uses the
// CIPD service to fetch packages.
//
// Packages that use the CIPD loader use the CIPD package name as their Path
// and a CIPD version/tag/ref as their Version.
type PackageLoader struct {
	// Options are additional client options to use when generating CIPD clients.
	Options cipd.ClientOptions

	// Template, if not nil, is a callback that will return additional CIPD
	// package template parameters. These may be derived from the VirtualEnv's
	// runtime environment.
	//
	// For example, if a user wanted to include the Python PEP425 tag version
	// as a CIPD template variable, they could include a "py_pep425_tag"
	// template parameter.
	Template TemplateFunc
}

var _ venv.PackageLoader = (*PackageLoader)(nil)

// Resolve implements venv.PackageLoader.
//
// The resulting packages slice will be updated in-place with the resolved
// package name and instance ID.
func (pl *PackageLoader) Resolve(c context.Context, e *vpython.Environment) error {
	spec := e.Spec
	if spec == nil {
		return nil
	}

	expander, err := pl.expanderForTags(c, e.Pep425Tag)
	if err != nil {
		return err
	}

	// Generate CIPD client options. If no root is provided, use a temporary root.
	if pl.Options.Root != "" {
		return pl.resolveWithOpts(c, pl.Options, expander, spec)
	}

	td := filesystem.TempDir{
		Prefix: "vpython_cipd",
		CleanupErrFunc: func(tdir string, err error) {
			logging.WithError(err).Warningf(c, "Failed to clean up CIPD temporary directory [%s]", tdir)
		},
	}
	return td.With(func(tdir string) error {
		opts := pl.Options
		opts.Root = tdir

		return pl.resolveWithOpts(c, opts, expander, spec)
	})
}

// resolveWithOpts resolves the specified packages.
//
// The supplied spec is updated with the resolved packages.
func (pl *PackageLoader) resolveWithOpts(c context.Context, opts cipd.ClientOptions,
	expander common.TemplateExpander, spec *vpython.Spec) error {

	logging.Debugf(c, "Resolving CIPD packages in root [%s]:", opts.Root)
	ef, packages := specToEnsureFile(spec)

	// Log our unresolved packages. Note that "specToEnsureFile" only creates
	// a subdir entry for the root (""), so we don't need to deterministically
	// iterate over the full map.
	if logging.IsLogging(c, logging.Debug) {
		for _, pkg := range ef.PackagesBySubdir[""] {
			logging.Debugf(c, "\tUnresolved package: %s", pkg)
		}
	}

	client, err := cipd.NewClient(opts)
	if err != nil {
		return errors.Annotate(err, "failed to generate CIPD client").Err()
	}

	// Start a CIPD client batch.
	client.BeginBatch(c)
	defer client.EndBatch(c)

	// Resolve our ensure file.
	resolved, err := ef.ResolveWith(func(pkg, vers string) (common.Pin, error) {
		pin, err := client.ResolveVersion(c, pkg, vers)
		if err != nil {
			return pin, errors.Annotate(err, "failed to resolve package %q at version %q", pkg, vers).Err()
		}

		logging.Fields{
			"package": pkg,
			"version": vers,
		}.Debugf(c, "Resolved package to: %s", pin)
		return pin, nil
	}, expander)
	if err != nil {
		return err
	}

	// Write the results to "packages". All of them should have been installed
	// into the root subdir.
	for i, pkg := range resolved.PackagesBySubdir[""] {
		packages[i].Name = pkg.PackageName
		packages[i].Version = pkg.InstanceID
	}
	return nil
}

// Ensure implement venv.PackageLoader.
//
// The packages must be valid (PackageIsComplete). If they aren't, Ensure will
// panic.
//
// The CIPD client that is used for the operation is generated from the supplied
// options, opts.
func (pl *PackageLoader) Ensure(c context.Context, root string, packages []*vpython.Spec_Package) error {
	pins, err := packagesToPins(packages)
	if err != nil {
		return errors.Annotate(err, "failed to convert packages to CIPD pins").Err()
	}
	pinSlice := common.PinSliceBySubdir{
		"": pins,
	}

	// Generate a CIPD client. Use the supplied root.
	opts := pl.Options
	opts.Root = root
	client, err := cipd.NewClient(opts)
	if err != nil {
		return errors.Annotate(err, "failed to generate CIPD client").Err()
	}

	// Start a CIPD client batch.
	client.BeginBatch(c)
	defer client.EndBatch(c)

	actionMap, err := client.EnsurePackages(c, pinSlice, false)
	if err != nil {
		return errors.Annotate(err, "failed to install CIPD packages").Err()
	}
	if len(actionMap) > 0 {
		errorCount := 0
		for root, action := range actionMap {
			errorCount += len(action.Errors)
			for _, err := range action.Errors {
				logging.Errorf(c, "CIPD root %q action %q for pin %q encountered error: %s", root, err.Action, err.Pin, err)
			}
		}
		if errorCount > 0 {
			return errors.Reason("CIPD package installation encountered %d error(s)", errorCount).Err()
		}
	}
	return nil
}

// Verify implements venv.PackageLoader.
func (pl *PackageLoader) Verify(c context.Context, sp *vpython.Spec, tags []*vpython.PEP425Tag) error {
	client, err := cipd.NewClient(pl.Options)
	if err != nil {
		return errors.Annotate(err, "failed to generate CIPD client").Err()
	}

	verify := cipd.Verifier{
		Client: client,
	}

	// Build an Ensure file for our specification under each tag and register it
	// with our Verifier.
	ensureFileErrors := 0
	for _, tag := range tags {
		tagSlice := []*vpython.PEP425Tag{tag}

		tagSpec := sp.Clone()
		if err := spec.NormalizeSpec(tagSpec, tagSlice); err != nil {
			return errors.Annotate(err, "failed to normalize spec for %q", tag).Err()
		}

		// Convert our spec into an ensure file.
		ef, _ := specToEnsureFile(tagSpec)

		expander, err := pl.expanderForTags(c, tagSlice)
		if err != nil {
			logging.Errorf(c, "Failed to generate template expander for: %s", tag.TagString())
			ensureFileErrors++
			continue
		}

		if err := verify.VerifyEnsureFile(c, ef, expander); err != nil {
			logging.Errorf(c, "Failed to verify package set for: %s", tag.TagString())
			ensureFileErrors++
		}
	}

	failed := false
	if ensureFileErrors > 0 {
		logging.Errorf(c, "Spec could not be resolved for %d tag(s).", ensureFileErrors)
		failed = true
	}

	// Verify all registered package sets.
	result := verify.Result()
	if result.HasErrors() {
		logging.Errorf(c, "Package verification failed to verify %d package(s) and %d pin(s).",
			len(result.InvalidPackages), len(result.InvalidPins))

		for _, pkg := range result.InvalidPackages {
			logging.Errorf(c, "Could not verify package: %s", pkg)
		}

		for _, pin := range result.InvalidPins {
			logging.Errorf(c, "Could not verify pin: %s", pin)
		}

		failed = true
	}

	if failed {
		return errors.New("verification failed")
	}

	logging.Infof(c, "Successfully verified %d package(s) and %d pin(s).", result.NumPackages, result.NumPins)
	return nil
}

// specToEnsureFile translates the packages named in spec into a CIPD ensure
// file.
//
// It returns an ensure file, ef, containing a specification that loads the
// contents of each package into the CIPD root (""). It also returns packages,
// a slice of pointers to spec's "vpython.Spec_Package" entries. Each PackageDef
// index in ef corresponds to the same index source vpython.Spec_Package from
// spec.
func specToEnsureFile(spec *vpython.Spec) (ef *ensure.File, packages []*vpython.Spec_Package) {
	// Create a single package list. Our VirtualEnv will be index 0 (need
	// this so we can back-port it into its VirtualEnv property).
	//
	// These will be updated to their resolved values in-place.
	packages = make([]*vpython.Spec_Package, 1, 1+len(spec.Wheel))
	packages[0] = spec.Virtualenv
	packages = append(packages, spec.Wheel...)

	pslice := make(ensure.PackageSlice, len(packages))
	for i, pkg := range packages {
		pslice[i] = ensure.PackageDef{
			PackageTemplate:   pkg.Name,
			UnresolvedVersion: pkg.Version,
		}
	}

	ef = &ensure.File{
		PackagesBySubdir: map[string]ensure.PackageSlice{"": pslice},
	}
	return
}

func (pl *PackageLoader) expanderForTags(c context.Context, tags []*vpython.PEP425Tag) (common.TemplateExpander, error) {
	// Build our aggregate template parameters. We prefer our template parameters
	// over the local system parameters. This allows us to override CIPD template
	// parameters elsewhere, and in the production case we will not override
	// any CIPD template parameters.
	expander := common.DefaultTemplateExpander()
	if pl.Template != nil {
		loaderTemplate, err := pl.Template(c, tags)
		if err != nil {
			return nil, errors.Annotate(err, "failed to get CIPD template arguments").Err()
		}
		for k, v := range loaderTemplate {
			expander[k] = v
		}
	}
	return expander, nil
}

func packagesToPins(packages []*vpython.Spec_Package) ([]common.Pin, error) {
	pins := make([]common.Pin, len(packages))
	for i, pkg := range packages {
		pins[i] = common.Pin{
			PackageName: pkg.Name,
			InstanceID:  pkg.Version,
		}
	}
	return pins, nil
}
