// Copyright 2016 The LUCI Authors.
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

package module

import (
	"golang.org/x/net/context"
)

// RawInterface is the interface for all of the package methods which normally
// would be in the 'module' package.
type RawInterface interface {
	List() ([]string, error)
	NumInstances(module, version string) (int, error)
	SetNumInstances(module, version string, instances int) error
	Versions(module string) ([]string, error)
	DefaultVersion(module string) (string, error)
	Start(module, version string) error
	Stop(module, version string) error
}

// List lists the names of modules belonging to this application.
func List(c context.Context) ([]string, error) {
	return Raw(c).List()
}

// NumInstances returns the number of instances servicing the specified
// module/version.
//
// If module or version is the empty string, it means the default.
func NumInstances(c context.Context, module, version string) (int, error) {
	return Raw(c).NumInstances(module, version)
}

// SetNumInstances sets the number of instances of a given module/version.
//
// If module or version is the empty string, it means the default.
func SetNumInstances(c context.Context, module, version string, instances int) error {
	return Raw(c).SetNumInstances(module, version, instances)
}

// Versions returns the names of versions for the specified module.
//
// If module is the empty string, it means the default.
func Versions(c context.Context, module string) ([]string, error) {
	return Raw(c).Versions(module)
}

// DefaultVersion returns the name of the default version for the specified
// module.
//
// If module is the empty string, it means the default.
func DefaultVersion(c context.Context, module string) (string, error) {
	return Raw(c).DefaultVersion(module)
}

// Start starts the specified module/version.
//
// If module or version is the empty string, it means the default.
func Start(c context.Context, module, version string) error {
	return Raw(c).Start(module, version)
}

// Stop stops the specified module/version.
//
// If module or version is the empty string, it means the default.
func Stop(c context.Context, module, version string) error {
	return Raw(c).Stop(module, version)
}
