// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cipd

import (
	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/venv"

	"github.com/luci/luci-go/cipd/client/cipd"
	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/ensure"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"golang.org/x/net/context"
)

// PackageLoader is an implementation of venv.PackageLoader that uses the
// CIPD service to fetch packages.
//
// Packages that use the CIPD loader use the CIPD package name as their Path
// and a CIPD version/tag/ref as their Version.
type PackageLoader struct {
	// Options are additional client options to use when generating CIPD clients.
	Options cipd.ClientOptions
}

var _ venv.PackageLoader = (*PackageLoader)(nil)

// Resolve implements venv.PackageLoader.
//
// The resulting packages slice will be updated in-place with the resolved
// package name and instance ID.
func (pl *PackageLoader) Resolve(c context.Context, root string, packages []*vpython.Spec_Package) error {
	if len(packages) == 0 {
		return nil
	}

	logging.Debugf(c, "Resolving CIPD packages:")
	pslice := make(ensure.PackageSlice, len(packages))
	for i, pkg := range packages {
		pslice[i] = ensure.PackageDef{
			PackageTemplate:   pkg.Name,
			UnresolvedVersion: pkg.Version,
		}

		logging.Debugf(c, "\tUnresolved package: %s", pslice[i])
	}

	ef := ensure.File{
		PackagesBySubdir: map[string]ensure.PackageSlice{"": pslice},
	}

	// Generate a CIPD client. Use the supplied root.
	opts := pl.Options
	opts.Root = root
	client, err := cipd.NewClient(opts)
	if err != nil {
		return errors.Annotate(err).Reason("failed to generate CIPD client").Err()
	}

	// Start a CIPD client batch.
	client.BeginBatch(c)
	defer client.EndBatch(c)

	// Resolve our ensure file.
	resolved, err := ef.Resolve(func(pkg, vers string) (common.Pin, error) {
		pin, err := client.ResolveVersion(c, pkg, vers)
		if err != nil {
			return pin, errors.Annotate(err).Reason("failed to resolve package %(package)q at version %(version)q").
				D("package", pkg).
				D("version", vers).
				Err()
		}

		logging.Fields{
			"package": pkg,
			"version": vers,
		}.Debugf(c, "Resolved package to: %s", pin)
		return pin, nil
	})
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
		return errors.Annotate(err).Reason("failed to convert packages to CIPD pins").Err()
	}
	pinSlice := common.PinSliceBySubdir{
		"": pins,
	}

	// Generate a CIPD client. Use the supplied root.
	opts := pl.Options
	opts.Root = root
	client, err := cipd.NewClient(opts)
	if err != nil {
		return errors.Annotate(err).Reason("failed to generate CIPD client").Err()
	}

	// Start a CIPD client batch.
	client.BeginBatch(c)
	defer client.EndBatch(c)

	actionMap, err := client.EnsurePackages(c, pinSlice, false)
	if err != nil {
		return errors.Annotate(err).Reason("failed to install CIPD packages").Err()
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
			return errors.Reason("CIPD package installation encountered %(count)d error(s)").
				D("count", errorCount).
				Err()
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
