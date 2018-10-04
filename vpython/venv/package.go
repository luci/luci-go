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

package venv

import (
	"context"

	"go.chromium.org/luci/vpython/api/vpython"
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

	// Verify verifies that all listed packages can be sufficiently resolved
	// for each supplied PEP425 tag.
	//
	// "spec" may be mutated during verification.
	Verify(c context.Context, spec *vpython.Spec, tags []*vpython.PEP425Tag) error
}
