// Copyright 2015 The LUCI Authors.
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

package isolated

// IsolatedFormatVersion is version of *.isolated file format. Put into JSON.
const IsolatedFormatVersion = "1.4"

// ReadOnlyValue defines permissions on isolated files.
type ReadOnlyValue int

const (
	// Writeable means that both files and directories are writeable. This should
	// not be used.
	Writeable ReadOnlyValue = 0
	// FilesReadOnly means that files are read only but directories are
	// writeable. This permits the process executed to create temporary files in
	// the current directory. This is the recommended value and this is the
	// default value.
	FilesReadOnly ReadOnlyValue = 1
	// DirsReadOnly means that both files and directories are read-only. This
	// enforces strict no-junk policy that the process cannot create files in the
	// temporary mapped directory.
	DirsReadOnly ReadOnlyValue = 2
)

// Algorithm is the value for Algo.
const Algorithm = "sha-1"

// FileType describes the type of file being isolated.
type FileType string

const (
	// Basic represents normal files. It is the default type.
	Basic FileType = "basic"

	// ArArchive represents an ar archive containing a large number of small files.
	ArArchive FileType = "ar"

	// TarArchive represents a tar archive containing a large number of small files.
	TarArchive FileType = "tar"
)

// File describes a single file referenced by content in a .isolated file.
//
// For regular files, the Digest, Mode, and Size fields should be set, and the
// Type field should be set for non-basic files.
// For symbolic links, only the Link field should be set.
type File struct {
	Digest HexDigest `json:"h,omitempty"`
	Link   *string   `json:"l,omitempty"`
	Mode   *int      `json:"m,omitempty"`
	Size   *int64    `json:"s,omitempty"`
	Type   FileType  `json:"t,omitempty"`
}

// BasicFile returns a File populated for a basic file.
func BasicFile(d HexDigest, mode int, size int64) File {
	return File{
		Digest: d,
		Mode:   &mode,
		Size:   &size,
	}
}

// SymLink returns a File populated for a symbolic link.
func SymLink(link string) File {
	return File{
		Link: &link,
	}
}

// TarFile returns a file populated for a tar archive file.
func TarFile(d HexDigest, mode int, size int64) File {
	return File{
		Digest: d,
		Mode:   &mode,
		Size:   &size,
		Type:   TarArchive,
	}
}

// Isolated is the data from a JSON serialized .isolated file.
type Isolated struct {
	Algo        string          `json:"algo"` // Must be "sha-1"
	Command     []string        `json:"command,omitempty"`
	Files       map[string]File `json:"files,omitempty"`
	Includes    HexDigests      `json:"includes,omitempty"`
	ReadOnly    *ReadOnlyValue  `json:"read_only,omitempty"`
	RelativeCwd string          `json:"relative_cwd,omitempty"`
	Version     string          `json:"version"`
}

// New returns a new Isolated with the default Algo and Version.
func New() *Isolated {
	return &Isolated{
		Algo:    Algorithm,
		Version: IsolatedFormatVersion,
		Files:   map[string]File{},
	}
}
