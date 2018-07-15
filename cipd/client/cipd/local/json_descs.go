// Copyright 2014 The LUCI Authors.
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

package local

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
)

const (
	// SiteServiceDir is a name of the directory inside an installation root
	// reserved for cipd stuff.
	SiteServiceDir = ".cipd"

	// PackageServiceDir is a name of the directory inside the package
	// reserved for cipd stuff.
	PackageServiceDir = ".cipdpkg"

	// ManifestName is a full name of the manifest file inside the package.
	ManifestName = PackageServiceDir + "/manifest.json"

	// ManifestFormatVersion is a version to write to the manifest file.
	ManifestFormatVersion = "1.1"
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

// ManifestMode is used to indicate presence of absence of manifest when calling
// various functions.
//
// Just to improve code readability, since Func(..., WithManifest) is less
// cryptic than Func(..., true).
type ManifestMode bool

const (
	// WithoutManifest indicates the function should skip manifest.
	WithoutManifest ManifestMode = false
	// WithManifest indicates the function should handle manifest.
	WithManifest ManifestMode = true
)

// Manifest defines structure of manifest.json file.
type Manifest struct {
	FormatVersion string      `json:"format_version"`
	PackageName   string      `json:"package_name"`
	VersionFile   string      `json:"version_file,omitempty"` // where to put JSON with info about deployed package
	InstallMode   InstallMode `json:"install_mode,omitempty"` // how to install: "copy" or "symlink"
	Files         []FileInfo  `json:"files,omitempty"`        // present only in deployed manifest
}

// FileInfo is JSON-ish struct with info extracted from File interface.
type FileInfo struct {
	// Name is slash separated file path relative to a package root.
	Name string `json:"name"`

	// Size is a size of the file. 0 for symlinks.
	Size uint64 `json:"size"`

	// Executable is true if the file is executable.
	//
	// Only used for Linux\Mac archives. False for symlinks.
	Executable bool `json:"executable,omitempty"`

	// Writable is true if the file is user-writable.
	Writable bool `json:"writable,omitempty"`

	// ModTime is Unix timestamp with modification time of the file as it is set
	// inside CIPD package.
	//
	// May be 0 if the package was built without preserving the modification
	// times.
	ModTime int64 `json:"modtime,omitempty"`

	// WinAttrs is a string representation of extra windows file attributes.
	//
	// Only used for Win archives.
	WinAttrs string `json:"win_attrs,omitempty"`

	// Symlink is a path the symlink points to or "" if the file is not a symlink.
	Symlink string `json:"symlink,omitempty"`
}

// VersionFile describes JSON file with package version information that's
// deployed to a path specified in 'version_file' attribute of the manifest.
type VersionFile struct {
	PackageName string `json:"package_name"`
	InstanceID  string `json:"instance_id"`
}

// ValidateInstallMode returns non nil if install mode is invalid.
//
// Valid modes are: "" (client will pick platform default), "copy"
// (aka InstallModeCopy), "symlink" (aka InstallModeSymlink).
func ValidateInstallMode(mode InstallMode) error {
	if mode == "" || mode == InstallModeCopy || mode == InstallModeSymlink {
		return nil
	}
	return fmt.Errorf("invalid install mode %q", mode)
}

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

// readManifest reads and decodes manifest JSON from io.Reader.
func readManifest(r io.Reader) (manifest Manifest, err error) {
	blob, err := ioutil.ReadAll(r)
	if err == nil {
		err = json.Unmarshal(blob, &manifest)
	}
	return
}

// writeManifest encodes and writes manifest JSON to io.Writer.
func writeManifest(m *Manifest, w io.Writer) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

const (
	// descriptionName is a name of the description file inside the package.
	descriptionName = "description.json"
)

// Description defines the structure of the description.json file located at
// .cipd/pkgs/<foo>/description.json.
type Description struct {
	Subdir      string `json:"subdir,omitempty"`
	PackageName string `json:"package_name,omitempty"`
}

// readDescription reads and decodes description JSON from io.Reader.
func readDescription(r io.Reader) (desc *Description, err error) {
	blob, err := ioutil.ReadAll(r)
	if err == nil {
		err = json.Unmarshal(blob, &desc)
	}
	return
}

// writeDescription encodes and writes description JSON to io.Writer.
func writeDescription(d *Description, w io.Writer) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
