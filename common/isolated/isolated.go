// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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

// File describes a single file referenced by content in a .isolated file.
//
// Either one of Size or Link can be set.
type File struct {
	Digest HexDigest `json:"h,omitempty"`
	Link   *string   `json:"l,omitempty"`
	Mode   *int      `json:"m,omitempty"`
	Size   *int64    `json:"s,omitempty"`
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
