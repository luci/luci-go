// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"errors"
)

// PackageManager represents the management interface for packages to provide
// a place for storing and retrieving packages.
type PackageManager interface {
	// Get(id) returns the handler for the package.
	Get(id string) PackageHandler
}

var (
	// ErrPackageNotExist returns when trying to increase the reference of a
	// non-existent package.
	ErrPackageNotExist = errors.New("package does not exist")
)

// PackageHandler is the interface for a handler in the storage. The content of
// a package can be built by calling Build(func(Package) error) error, which
// will make package available if successful and can be referenced by other
// packages. Package shouldn't be modified after build.
type PackageHandler interface {
	// OutputDirectory() returns the output directory.
	OutputDirectory() string

	// LoggingDirectory() returns the logging directory.
	LoggingDirectory() string

	// Build(buildFunc) makes packages available in the storage.
	// It's responsible for:
	// - Hold exclusive lock to the package during the build.
	// - Ensure build only happens once unless the package is removed.
	// - Check the remote cache server (if possible).
	// - Set up the build environment (e.g. create output directory).
	// - Mark package available if build function successfully returns.
	// Calling build function is expected to trigger the actual build iff package
	// isn't available yet.
	Build(builder func() error) error

	// TryRemove(), IncRef(), DecRef() are the interface for removable packages.
	// If removing package is not supported by PackageManager, TryRemove() will
	// always return false.
	// IncRef() and DecRef() references/dereferences the package to prevent
	// package from being removed, thus they can be no-op if the removal never
	// happens.

	// TryRemove() may remove the package only if there is no reference to it.
	TryRemove() (ok bool, err error)

	// Reference the package to prevent it from being removed while in use.
	// IncRef() updates the last access time of the package as well.
	// IncRef() only succeeds if the package is available.
	// Otherwise ErrPackageNotExist will be returned.
	IncRef() error
	DecRef() error
}
