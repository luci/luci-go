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

package cipdpkg

import (
	"encoding/json"
	"io"
	"io/ioutil"
)

const (
	// ServiceDir is a name of the directory inside the package zip file reserved
	// for cipd stuff.
	ServiceDir = ".cipdpkg"

	// ManifestName is a full name of the manifest file inside the package.
	ManifestName = ServiceDir + "/manifest.json"

	// ManifestFormatVersion is a version to write to the manifest file.
	ManifestFormatVersion = "1.1"
)

// Manifest defines structure of manifest.json file.
type Manifest struct {
	FormatVersion string      `json:"format_version"`
	PackageName   string      `json:"package_name"`
	VersionFile   string      `json:"version_file,omitempty"` // where to put JSON with info about deployed package
	InstallMode   InstallMode `json:"install_mode,omitempty"` // how to install: "copy" or "symlink"
	Files         []FileInfo  `json:"files,omitempty"`        // present only in deployed manifest
}

// FileInfo is a definition of a file inside a CIPD package.
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

// ReadManifest reads and decodes manifest JSON from io.Reader.
func ReadManifest(r io.Reader) (manifest Manifest, err error) {
	blob, err := ioutil.ReadAll(r)
	if err == nil {
		err = json.Unmarshal(blob, &manifest)
	}
	return
}

// WriteManifest encodes and writes manifest JSON to io.Writer.
func WriteManifest(m *Manifest, w io.Writer) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}
