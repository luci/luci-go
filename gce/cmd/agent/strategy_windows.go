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

package main

import (
	"os/exec"
	"syscall"
)

// setFlags sets flags which ensure the process starts detached in its own process group.
func setFlags(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{
		// https://msdn.microsoft.com/en-us/library/windows/desktop/ms684863.aspx
		// CREATE_NEW_PROCESS_GROUP: 0x200
		// DETACHED_PROCESS:         0x008
		CreationFlags: 0x200 | 0x8,
	}
}

// newStrategy returns a new Windows-specific PlatformStrategy.
func newStrategy() PlatformStrategy {
	return &WindowsStrategy{}
}
