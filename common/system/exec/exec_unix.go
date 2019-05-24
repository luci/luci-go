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

// +build !windows

package exec

import (
	osexec "os/exec"
	"syscall"
	"time"

	"go.chromium.org/luci/common/errors"
)

// ErrTimeout is error for timeout.
var ErrTimeout = errors.Reason("timeout").Err()

// Proc holds fileds for timeout support.
type Proc struct {
	cmd *osexec.Cmd

	waitCh chan error
}

// NewProc creates new process with cmd.
func NewProc(cmd *osexec.Cmd) *Proc {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}

	// Use process group to kill all child processes.
	cmd.SysProcAttr.Setpgid = true

	proc := &Proc{
		cmd:    cmd,
		waitCh: make(chan error),
	}

	return proc
}

// Start starts command.
func (p *Proc) Start() error {
	if err := p.cmd.Start(); err != nil {
		return err
	}

	go func() {
		p.waitCh <- p.cmd.Wait()
		close(p.waitCh)
	}()

	return nil
}

// Wait waits for timeout.
func (p *Proc) Wait(timeout time.Duration) error {
	select {
	case <-time.After(timeout):
		return ErrTimeout
	case err := <-p.waitCh:
		return err
	}
}

// Terminate sends SIGTERM.
func (p *Proc) Terminate() error {
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return errors.Annotate(err, "failed to send SIGTERM").Err()
	}
	return nil
}

// Kill sends SIGKILL.
func (p *Proc) Kill() error {
	if err := p.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		return errors.Annotate(err, "failed to send SIGKILL").Err()
	}
	return nil
}

// ExitCode returns exit code of process.
func (p *Proc) ExitCode() int {
	if ws, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		return ws.ExitStatus()
	}
	return p.cmd.ProcessState.ExitCode()
}
