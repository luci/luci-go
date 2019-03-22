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

// Alias some types from various CIPD packages to make public facing API look
// more uniform. We can't put all types directly into 'cipd' package due to
// dependency cycles.

import (
	"go.chromium.org/luci/cipd/client/cipd/deployer"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
)

// ParanoidMode specifies how paranoid EnsurePackages should be.
type ParanoidMode = deployer.ParanoidMode

const (
	// NotParanoid indicates that EnsurePackages should trust its metadata
	// directory: if a package is marked as installed there, it should be
	// considered correctly installed in the site root too.
	NotParanoid = deployer.NotParanoid

	// CheckPresence indicates that CheckDeployed should verify all files
	// that are supposed to be installed into the site root are indeed present
	// there, and reinstall ones that are missing.
	//
	// Note that it will not check file's content or file mode. Only its presence.
	CheckPresence = deployer.CheckPresence

	// CheckIntegrity indicates that CIPD client should verify all files installed
	// in the site root have correct content (based on their hashes).
	//
	// CIPD will use information from 'stat' to skip rechecking hashes all the
	// time. Only files modified (based on 'stat') since they were installed are
	// checked.
	CheckIntegrity = deployer.CheckIntegrity
)

// ManifestMode is used to indicate presence of absence of manifest when calling
// various functions.
//
// Just to improve code readability, since Func(..., WithManifest) is less
// cryptic than Func(..., true).
type ManifestMode = pkg.ManifestMode

const (
	// WithoutManifest indicates the function should skip manifest.
	WithoutManifest = pkg.WithoutManifest
	// WithManifest indicates the function should handle manifest.
	WithManifest = pkg.WithManifest
)
