// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ctxcmd

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/net/context"
)

// Error is the type of error returned by Wait().
type cmdResult struct {
	// err is the acutal error to return from Wait.
	err error
	// state is the process' returned state.
	state *os.ProcessState
	// procErr is the process' returned error. Depending on whether the process
	// was cancelled, this may equal err.
	procErr error
}

// CtxCmd is a wrapper around an exec.Cmd that responds to a Context's
// cancellation by terminating the process.
//
// A Cmd managed by CtxCmd should not have its Run, Start, or Wait methods
// used.
type CtxCmd struct {
	*exec.Cmd

	// ProcessError is the error returned by the process. It is populated when
	// Run() or Wait() exits.
	//
	// This may differ from the return value of Run() or Wait() if the process
	// was cancelled.
	ProcessError error

	// CancelSignal, if not nil, is the signal that is sent to the process to
	// terminate it when it is cancelled. If nil, the os.Kill signal will be sent.
	CancelSignal os.Signal

	waitC chan *cmdResult

	// (Testing) if not nil, write a value here when the process has finished.
	testProcFinishedC chan struct{}
}

// Run starts the process, blocking until it has exited. It is the analogue to
// the underlying Cmd.Run().
//
// If the supplied Context is cancelled (deadline, cancel, etc.), the process
// will be killed prematurely.
func (cc *CtxCmd) Run(c context.Context) error {
	if err := cc.Start(c); err != nil {
		return err
	}
	return cc.Wait()
}

// Start starts the process and returns. It is the analogue to the underlying
// Cmd.Start().
//
// The process should then be reaped via Wait().
//
// If the supplied Context is cancelled (deadline, cancel, etc.), the process
// will be killed prematurely.
func (cc *CtxCmd) Start(c context.Context) error {
	// Check if the Context has been cancelled before wasting our time executing.
	select {
	case <-c.Done():
		return c.Err()

	default:
		break
	}

	if err := cc.Cmd.Start(); err != nil {
		return err
	}

	// Begin Waiting on the process. This is asynchronous and will ultimately
	// feed back into Wait().
	finishedC := make(chan *cmdResult)
	go func() {
		var r cmdResult
		defer func() {
			finishedC <- &r
			close(finishedC)
		}()

		r.state, r.procErr = cc.Process.Wait()
	}()

	// Start a monitor to wait on Context cancel or process exit.
	waitC := make(chan *cmdResult)
	go func() {
		// Write the process result to "waitC".
		var r *cmdResult
		defer func() {
			defer close(waitC)

			// (Testing)
			if cc.testProcFinishedC != nil {
				cc.testProcFinishedC <- struct{}{}
			}

			waitC <- r
		}()

		select {
		case r = <-finishedC:
			r.err = r.procErr

		case <-c.Done():
			// Make sure the process didn't already finish.
			select {
			case r = <-finishedC:
				r.err = r.procErr
				return

			default:
				// Go ahead and kill/cancel.
				break
			}

			cc.cancel()
			r = <-finishedC
			r.err = c.Err()
		}
	}()

	cc.waitC = waitC
	return nil
}

// Wait is the cancellable analogue of Cmd's Wait(). It blocks until the process
// exits.
//
// If the Context was cancelled, this will still block until the process exits.
func (cc *CtxCmd) Wait() error {
	r := <-cc.waitC
	cc.waitC = nil

	// Record our process' immediate results.
	cc.ProcessState, cc.ProcessError = r.state, r.procErr

	// If our process ran, but exited with a non-zero error code, return an error
	// (follows contract of exec.Cmd's Wait)
	if r.procErr == nil {
		// Forge an ExitError.
		cc.ProcessError = &exec.ExitError{cc.ProcessState}
		if r.err == nil && !r.state.Success() {
			// If there wasn't a higher-level error, exit with the process error.
			return cc.ProcessError
		}
	}
	return r.err
}

func (cc *CtxCmd) cancel() error {
	if cc.CancelSignal != nil {
		return cc.Signal(cc.CancelSignal)
	}
	return cc.Kill()
}

// Kill sends a kill signal to the underlying process. This only works if the
// Process is currently running.
func (cc *CtxCmd) Kill() error {
	return cc.Cmd.Process.Kill()
}

// Signal sends a signal to the underlying process. This only works if the
// Process is currently running.
func (cc *CtxCmd) Signal(sig os.Signal) error {
	return cc.Cmd.Process.Signal(sig)
}

// ExitCode returns the process exit code given an error. If no exit code is
// present, 0 will be returned.
func ExitCode(err error) (int, bool) {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.Sys().(syscall.WaitStatus).ExitStatus(), true
	}
	return 0, false
}
