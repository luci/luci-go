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

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/venv"

	"github.com/luci/luci-go/cipd/client/cipd"
	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/ensure"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/system/filesystem"
)

// TemplateFunc builds a set of template parameters to augment the default CIPD
// parameter set with.
type TemplateFunc func(context.Context, *vpython.Environment) (map[string]string, error)

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

	// Build our aggregate template parameters. We prefer our template parameters
	// over the local system parameters. This allows us to override CIPD template
	// parameters elsewhere, and in the production case we will not override
	// any CIPD template parameters.
	template := common.TemplateArgs()
	if pl.Template != nil {
		loaderTemplate, err := pl.Template(c, e)
		if err != nil {
			return errors.Annotate(err, "failed to get CIPD template arguments").Err()
		}
		for k, v := range loaderTemplate {
			template[k] = v
		}
	}

	// Create a single package list. Our VirtualEnv will be index 0 (need
	// this so we can back-port it into its VirtualEnv property).
	//
	// These will be updated to their resolved values in-place.
	packages := make([]*vpython.Spec_Package, 1, 1+len(spec.Wheel))
	packages[0] = spec.Virtualenv
	packages = append(packages, spec.Wheel...)

	// Generate CIPD client options. If no root is provided, use a temporary root.
	if pl.Options.Root != "" {
		return pl.resolveWithOpts(c, pl.Options, template, packages)
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

		return pl.resolveWithOpts(c, opts, template, packages)
	})
}

// resolveWithOpts resolves the specified packages.
func (pl *PackageLoader) resolveWithOpts(c context.Context, opts cipd.ClientOptions, template map[string]string,
	packages []*vpython.Spec_Package) error {

	logging.Debugf(c, "Resolving CIPD packages in root [%s]:", opts.Root)
	pslice := make(ensure.PackageSlice, len(packages))
	for i, pkg := range packages {
		pslice[i] = ensure.PackageDef{
			PackageTemplate:   pkg.Name,
			UnresolvedVersion: pkg.Version,
		}

		logging.Debugf(c, "\tUnresolved package: %s", pkg)
	}
	ef := ensure.File{
		PackagesBySubdir: map[string]ensure.PackageSlice{"": pslice},
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
	}, template)
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
