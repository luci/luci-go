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
	"os/exec"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"

	"go.chromium.org/luci/luciexe"
)

// Subprocess represents a running luciexe.
type Subprocess struct {
	Step        *bbpb.Step
	collectPath string

	ctx context.Context
	cmd *exec.Cmd

	closeChannels chan<- struct{}
	allClosed     <-chan error

	waitOnce           sync.Once
	build              *bbpb.Build
	err                errors.MultiError
	firstDeadlineEvent atomic.Value // stores lucictx.DeadlineEvent
}

// Start launches a binary implementing the luciexe protocol and returns
// immediately with a *Subprocess.
//
// Args:
//   - ctx will be used for deadlines/cancellation of the started luciexe.
//   - luciexeArgs[0] must be the full absolute path to the luciexe binary.
//   - input must be the Build message you wish to pass to the luciexe binary.
//   - opts is optional (may be nil to take all defaults)
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
func Start(ctx context.Context, luciexeArgs []string, input *bbpb.Build, opts *Options) (*Subprocess, error) {
	initialBuildData, err := proto.Marshal(mkInitialBuild(ctx, input))
	if err != nil {
		return nil, errors.Annotate(err, "marshalling initial Build").Err()
	}

	launchOpts, _, err := opts.rationalize(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "normalizing options").Err()
	}

	closeChannels := make(chan struct{})
	allClosed := make(chan error)
	go func() {
		select {
		case <-ctx.Done():
		case <-closeChannels:
		}
		err := errors.NewLazyMultiError(2)
		if serr := launchOpts.stdout.Close(); serr != nil {
			err.Assign(0, errors.Annotate(serr, "closing stdout").Err())
		}
		if serr := launchOpts.stderr.Close(); serr != nil {
			err.Assign(1, errors.Annotate(serr, "closing stderr").Err())
		}
		allClosed <- err.Get()
	}()

	args := make([]string, 0, len(luciexeArgs)+len(launchOpts.args)-1)
	args = append(args, luciexeArgs[1:]...)
	args = append(args, launchOpts.args...)

	cmd := exec.CommandContext(ctx, luciexeArgs[0], args...)
	cmd.Env = launchOpts.env.Sorted()
	cmd.Dir = launchOpts.workDir
	cmd.Stdin = bytes.NewBuffer(initialBuildData)
	cmd.Stdout = launchOpts.stdout
	cmd.Stderr = launchOpts.stderr
	setSysProcAttr(cmd)

	// NOTE: Technically this is racy; if `ctx` expires immediately after we check
	// this, then we'll return no error, but CommandContext will kill the process
	// straight away.
	//
	// However, in tests, when you've misconfigured the Deadline on ctx (e.g.
	// using a fake clock), this check is generally not racy, and can provide
	// a very valuable hint that's clearer than getting an error from Wait().
	if err := ctx.Err(); err != nil {
		// clean up stdout/stderr
		close(closeChannels)
		<-allClosed
		return nil, errors.Annotate(err, "prior to starting subprocess").Err()
	}

	if err := cmd.Start(); err != nil {
		// clean up stdout/stderr
		close(closeChannels)
		<-allClosed
		return nil, errors.Annotate(err, "launching luciexe").Err()
	}

	s := &Subprocess{
		Step:        launchOpts.step,
		collectPath: launchOpts.collectPath,
		ctx:         ctx,
		cmd:         cmd,

		closeChannels: closeChannels,
		allClosed:     allClosed,
	}

	if deadlineEvtCh := lucictx.SoftDeadlineDone(ctx); deadlineEvtCh != nil {
		go func() {
			select {
			case <-closeChannels:
				// luciexe subprocess exits normally
			case evt := <-deadlineEvtCh:
				s.firstDeadlineEvent.Store(evt)
				logging.Warningf(ctx, "got SoftDeadline event %s", evt)

				if evt == lucictx.InterruptEvent || evt == lucictx.TimeoutEvent {
					logging.Infof(ctx, "sending Terminate")
					if err := s.terminate(); err != nil {
						logging.Errorf(ctx, "failed to terminate luciexe subprocess, reason: %s", err)
					}
				}
				// if evt == lucictx.ClosureEvent, it means that ctx.Done() is closed,
				// which means that CommandContext has already sent Kill to the process.
			}
		}()
	}
	return s, nil
}

// Wait waits for the subprocess to terminate.
//
// If Options.CollectOutput (default: false) was specified, this will return the
// final Build message, as reported by the luciexe.
//
// In all cases, finalBuild.StatusDetails will indicate if this Subprocess
// instructed the luciexe to stop via timeout from deadlineEvtCh passed to Start.
//
// If you wish to cancel the subprocess (e.g. due to a timeout or deadline),
// make sure to pass a cancelable/deadline context to Start().
//
// Calling this multiple times is OK; it will return the same values every time.
func (s *Subprocess) Wait() (finalBuild *bbpb.Build, err error) {
	s.waitOnce.Do(func() {
		defer func() {
			if s.build == nil {
				s.build = &bbpb.Build{}
			}
			// If our process saw a timeout or we think we're in the grace period now,
			// then we indicate that here.
			if s.firstDeadlineEvent.Load() == lucictx.TimeoutEvent {
				proto.Merge(s.build, &bbpb.Build{
					StatusDetails: &bbpb.StatusDetails{
						Timeout: &bbpb.StatusDetails_Timeout{},
					},
				})
			}
		}()

		defer func() {
			var errMsg string

			// We need to check both evt and ctxErr since they can race.
			ctxErr := s.ctx.Err()
			evt := s.firstDeadlineEvent.Load()
			switch {
			case evt == lucictx.InterruptEvent:
				errMsg = "luciexe process is interrupted"
			case evt == lucictx.TimeoutEvent || ctxErr == context.DeadlineExceeded:
				errMsg = "luciexe process timed out"
			case evt == lucictx.ClosureEvent || ctxErr == context.Canceled:
				errMsg = "luciexe process's context is cancelled"
			}

			if errMsg != "" {
				s.err.MaybeAdd(errors.New(errMsg))
			}
		}()
		// No matter what, we want to close stdout/stderr; if none of the other
		// return values have set `err`, it will be set to the result of closing
		// stdout/stderr.
		defer func() {
			close(s.closeChannels)
			s.err.MaybeAdd(<-s.allClosed)
		}()

		err := s.cmd.Wait()
		if err != nil {
			s.err = append(s.err, errors.Annotate(err, "waiting for luciexe").Err())
		}

		// Even if the Wait fails (e.g. process returns non-0 exit code, or other
		// issue), still try to read the build output.
		s.build, err = luciexe.ReadBuildFile(s.collectPath)
		s.err.MaybeAdd(err)
	})
	return s.build, s.err.AsError()
}

// fieldsToClear are a set of fields that MUST be cleared in the initial build
// to luciexe.
var fieldsToClear = stringset.NewFromSlice(
	"end_time",
	"status_details",
	"summary_markdown",
	"steps",
	"output",
	"update_time",
)

func mkInitialBuild(ctx context.Context, input *bbpb.Build) *bbpb.Build {
	ib := &bbpb.Build{}
	ibr := ib.ProtoReflect()
	input.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, val protoreflect.Value) bool {
		if !fieldsToClear.Has(string(field.Name())) {
			ibr.Set(field, val)
		}
		return true
	})
	now := clock.Now(ctx)
	if ib.CreateTime == nil {
		ib.CreateTime = timestamppb.New(now)
	}
	if ib.StartTime == nil {
		ib.StartTime = timestamppb.New(now)
	}
	ib.Status = bbpb.Status_STARTED
	return ib
}
