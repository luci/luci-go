// Copyright 2025 The LUCI Authors.
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

// Package terminal contains helpers for working with Unix-like terminals.
package terminal

import (
	"os"

	"golang.org/x/term"
)

// Caps are capabilities of the terminal.
type Caps struct {
	SupportsCursor bool // supports CursorUp and CursorDown control characters
}

// Enable switches the console attached to the given file (if any) into
// a terminal mode and returns its capabilities.
//
// Returns (nil, nil) if the given file is not associated with a terminal or
// it can't be switched into a terminal mode.
//
// On success returns a function that must be called once all terminal
// interactions are done.
//
// On Unix, this just checks the file is associated with a terminal and fetches
// some properties of this terminal. On Windows, it actually switches the
// console into a virtual terminal mode. The returned completion callback will
// switch it back into its original mode.
func Enable(f *os.File) (caps *Caps, done func()) {
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return nil, nil
	}
	return enableImpl(fd)
}
