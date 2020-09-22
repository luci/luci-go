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

package invoke

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/luciexe"

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// Subprocess represents a running luciexe.
type Subprocess struct {
	Step *bbpb.Step

	waitDone <-chan struct{}
	build    *bbpb.Build
	err      error
}

func launchTripleTapWatcher(ctx context.Context, cmd *exec.Cmd, cmdWaiter <-chan struct{}, gracePeriod time.Duration) {
	interruptCh := make(chan os.Signal, 1)
	signal.Notify(interruptCh, signals.Interrupts()...)

	go func() {
		defer func() {
			signal.Stop(interruptCh)
			close(interruptCh)
			_, _ = <-interruptCh
		}()

		defer killGroup(cmd)

		select {
		case <-ctx.Done():
			logging.Infof(ctx, "Got context cancelation.")
		case signum := <-interruptCh:
			logging.Infof(ctx, "Got interrupt: %s", signum)
		case <-cmdWaiter:
			return
		}

		t := time.NewTimer(gracePeriod)
		timeout := t.C
		if gracePeriod < 0 { // user asked us to wait forever
			timeout = nil
		}

		// We specifically do NOT use the LUCI clock package here; if context is
		// canceled, it will return immediately :(.
		interruptGroup(cmd)
		select {
		case <-timeout:
		case signum := <-interruptCh:
			logging.Infof(ctx, "Got interrupt: %s", signum)
		case <-cmdWaiter:
			return
		}
		if !t.Stop() {
			<-t.C
		}
		t.Reset(time.Duration(float64(gracePeriod) * 0.2))

		interruptGroup(cmd)
		select {
		case <-timeout:
		case signum := <-interruptCh:
			logging.Infof(ctx, "Got interrupt: %s", signum)
		case <-cmdWaiter:
			return
		}
		if !t.Stop() {
			<-t.C
		}
	}()

	return
}

// Start launches a binary implementing the luciexe protocol and returns
// immediately with a *Subprocess.
//
// Args:
//  * ctx will be used for deadlines/cancellation of the started luciexe.
//  * luciexeArgs[0] must be the full absolute path to the luciexe binary.
//  * input must be the Build message you wish to pass to the luciexe binary.
//  * opts is optional (may be nil to take all defaults)
//
// Callers MUST call Wait and/or cancel the context or this will leak handles
// for the process' stdout/stderr.
//
// This assumes that the current process is already operating within a "host
// application" environment. See "go.chromium.org/luci/luciexe" for details.
//
// The caller SHOULD immediately take Subprocess.Step, append it to the current
// Build state, and send that (e.g. using `exe.BuildSender`). Otherwise this
// luciexe's steps will not show up in the Build.
func Start(ctx context.Context, luciexeArgs []string, input *bbpb.Build, opts *Options) (subp *Subprocess, err error) {
	inputData, err := proto.Marshal(input)
	if err != nil {
		return nil, errors.Annotate(err, "marshalling input Build").Err()
	}

	launchOpts, _, err := opts.rationalize(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "normalizing options").Err()
	}

	args := make([]string, 0, len(luciexeArgs)+len(launchOpts.args)-1)
	args = append(args, luciexeArgs[1:]...)
	args = append(args, launchOpts.args...)

	// Don't do ContextCommand here, as we want to pass through signals.
	cmd := exec.Command(luciexeArgs[0], args...)
	cmd.Env = launchOpts.env.Sorted()
	cmd.Dir = launchOpts.workDir
	cmd.Stdin = bytes.NewBuffer(inputData)
	cmd.Stdout = launchOpts.stdout
	cmd.Stderr = launchOpts.stderr
	cmd.SysProcAttr = sysProcAttrs

	waitDone := make(chan struct{})

	if !opts.UnregisterSignalHandlers {
		blackHole := make(chan os.Signal)
		go func() {
			for range blackHole {
			}
		}()
		signal.Notify(blackHole, signals.Interrupts()...)
	}
	launchTripleTapWatcher(ctx, cmd, waitDone, launchOpts.gracePeriod)

	closeHandles := func() error {
		err := errors.NewLazyMultiError(2)
		err.Assign(0, errors.Annotate(launchOpts.stdout.Close(), "closing stdout").Err())
		err.Assign(1, errors.Annotate(launchOpts.stderr.Close(), "closing stderr").Err())
		return err.Get()
	}

	if err := cmd.Start(); err != nil {
		// clean up stdout/stderr
		closeHandles()
		return nil, errors.Annotate(err, "launching luciexe").Err()
	}

	subp = &Subprocess{
		Step:     launchOpts.step,
		waitDone: waitDone,
	}

	// Waiter thread.
	go func() {
		// No matter what, we want to close stdout/stderr; if none of the other
		// return values have set `err`, it will be set to the result of closing
		// stdout/stderr.
		defer func() {
			if closeErr := closeHandles(); subp.err == nil {
				subp.err = closeErr
			}
			close(waitDone)
		}()

		if subp.err = cmd.Wait(); subp.err != nil {
			subp.err = errors.Annotate(subp.err, "waiting for luciexe").Err()
			return
		}
		subp.build, subp.err = luciexe.ReadBuildFile(launchOpts.collectPath)
	}()

	return
}

// Wait waits for the subprocess to terminate.
//
// If Options.CollectOutput (default: false) was specified, this will return the
// final Build message, as reported by the luciexe.
//
// If you wish to cancel the subprocess (e.g. due to a timeout or deadline),
// make sure to pass a cancelable/deadline context to Start().
//
// Calling this multiple times is OK; it will return the same values every time.
func (s *Subprocess) Wait() (*bbpb.Build, error) {
	<-s.waitDone
	return s.build, s.err
}
