// Copyright 2023 The LUCI Authors.
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
//
//go:build !windows
// +build !windows

package internal

import (
	"fmt"
	"os"

	"github.com/xo/terminfo"
)

// EnableVirtualTerminal is a dummy function on unix systems that always
// returns supported=true.
func EnableVirtualTerminal() (supported bool, done func()) {
	return true, func() {}
}

// IsDumbTerminal determines if the current terminal supports CursorUp and CursorDown
// control characters.
func IsDumbTerminal() bool {
	ti, err := terminfo.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s, couldn't load terminal info from terminfo/\n", err.Error())
		return true
	}
	return ti.Strings[terminfo.CursorDown] == nil || ti.Strings[terminfo.CursorUp] == nil
}
