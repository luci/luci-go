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

package pkg

import (
	"runtime"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/common/cipderr"
)

// InstallMode defines how to install a package.
type InstallMode string

const (
	// InstallModeSymlink is default (for backward compatibility).
	//
	// In this mode all files are extracted to .cipd/*/... and then symlinked to
	// the site root directory. Version switch happens atomically.
	InstallModeSymlink InstallMode = "symlink"

	// InstallModeCopy is used when files should be directly copied.
	//
	// This mode is always used on Windows (and can be optionally) used on
	// other OSes. If installation is aborted midway, the package may end up
	// in inconsistent state.
	InstallModeCopy InstallMode = "copy"
)

// Set is called by 'flag' package when parsing command line options.
func (m *InstallMode) Set(value string) error {
	val := InstallMode(value)
	if err := ValidateInstallMode(val); err != nil {
		return err
	}
	*m = val
	return nil
}

// String is needed to conform to flag.Value interface.
func (m InstallMode) String() string {
	return string(m)
}

// ValidateInstallMode returns non nil if install mode is invalid.
//
// Valid modes are: "" (client will pick platform default), "copy"
// (aka InstallModeCopy), "symlink" (aka InstallModeSymlink).
func ValidateInstallMode(mode InstallMode) error {
	if mode == "" || mode == InstallModeCopy || mode == InstallModeSymlink {
		return nil
	}
	return cipderr.BadArgument.Apply(errors.
		Fmt("invalid install mode %q", mode))
}

// PickInstallMode validates the install mode and picks the correct default
// for the platform if no install mode is given.
func PickInstallMode(im InstallMode) (InstallMode, error) {
	switch {
	case runtime.GOOS == "windows":
		return InstallModeCopy, nil // Windows supports only 'copy' mode
	case im == "":
		return InstallModeSymlink, nil // default on other platforms
	}
	if err := ValidateInstallMode(im); err != nil {
		return "", err
	}
	return im, nil
}
