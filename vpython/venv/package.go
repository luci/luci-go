// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package venv

import (
	"golang.org/x/net/context"

	"github.com/luci/luci-go/vpython/api/vpython"
)

// PackageLoader loads package information from a specification file's Package
// message onto the local system.
//
// The PackageLoader instance is responsible for managing any caching,
// configuration, or setup required to operate.
type PackageLoader interface {
	// Resolve processes the packages defined in e, updating their fields to their
	// resolved values. Resolved packages must fully specify the package instance
	// that is being deployed, and will be used when determining the environment's
	// fingerprint (used for locking and naming).
	Resolve(c context.Context, e *vpython.Environment) error

	// Ensure installs the supplied packages into root.
	//
	// The packages will have been previously resolved via Resolve.
	Ensure(c context.Context, root string, packages []*vpython.Spec_Package) error
}
