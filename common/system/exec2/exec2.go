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

package exec2

import (
	"context"
	"os/exec"
	"time"

	"go.chromium.org/luci/common/errors"
)

// ErrTimeout is error for timeout.
var ErrTimeout = errors.Reason("timeout").Err()

// Cmd is like exec.Cmd, but has fields for timeout support.
type Cmd struct {
	*exec.Cmd

	waitCh chan error
}

// CommandContext is like exec.CommandContext, but it uses process group by
// default and supports timeout in Wait function.
func CommandContext(ctx context.Context, name string, arg ...string) *Cmd {
	cmd := &Cmd{
		Cmd:    exec.CommandContext(ctx, name, arg...),
		waitCh: make(chan error),
	}

	cmd.setupCmd()

	return cmd
}

// Start starts command.
func (c *Cmd) Start() error {
	if err := c.Cmd.Start(); err != nil {
		return err
	}

	go func() {
		c.waitCh <- c.Cmd.Wait()
		close(c.waitCh)
	}()

	return nil
}

// Wait waits for timeout.
func (c *Cmd) Wait(timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return ErrTimeout
	case err := <-c.waitCh:
		return err
	}
}

// Terminate sends SIGTERM.
func (c *Cmd) Terminate() error {
	return c.terminate()
}
