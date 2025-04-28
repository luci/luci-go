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
//
//go:build windows
// +build windows

package terminal

import (
	"golang.org/x/sys/windows"
)

func enableImpl(fd int) (*Caps, func()) {
	handle := windows.Handle(fd)

	var originalMode uint32
	err := windows.GetConsoleMode(handle, &originalMode)
	if err != nil {
		return nil, nil
	}

	err = windows.SetConsoleMode(handle, originalMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	if err != nil {
		windows.SetConsoleMode(handle, originalMode)
		return nil, nil
	}

	return &Caps{SupportsCursor: true}, func() {
		windows.SetConsoleMode(handle, originalMode)
	}
}
