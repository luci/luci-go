// Copyright 2019 The LUCI Authors.
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

//go:build !windows
// +build !windows

package exec2

import (
	"syscall"

	"go.chromium.org/luci/common/errors"
)

type attr struct{}

func (c *Cmd) setupCmd() {
	c.SysProcAttr = &syscall.SysProcAttr{
		// Use process group to kill all child processes.
		Setpgid: true,
	}
}

func (c *Cmd) start() error {
	return c.Cmd.Start()
}

func (c *Cmd) terminate() error {
	if err := c.Process.Signal(syscall.SIGTERM); err != nil {
		return errors.Fmt("failed to send SIGTERM: %w", err)
	}
	return nil
}

func (c *Cmd) kill() error {
	return c.Process.Kill()
}

func (c *Cmd) wait() error {
	return c.Cmd.Wait()
}
