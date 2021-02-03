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
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"

	bbpb "go.chromium.org/luci/buildbucket/proto"
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
	err                error
	firstDeadlineEvent atomic.Value // stores lucictx.DeadlineEvent
}

// Terminate sends SIGTERM on unix or CTRL+BREAK on windows to the luciexe
// process.
func (s *Subprocess) Terminate() error {
	if s != nil && s.cmd != nil && s.cmd.Process != nil {
		return s.terminiate()
	}
	return nil
}

// Kill kills the luciexe process.
func (s *Subprocess) Kill() error {
	if s != nil && s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Kill()
	}
	return nil
}

// Start launches a binary implementing the luciexe protocol and returns
// immediately with a *Subprocess.
//
// Args:
//  * ctx will be used for deadlines/cancellation of the started luciexe.
//  * luciexeArgs[0] must be the full absolute path to the luciexe binary.
//  * input must be the Build message you wish to pass to the luciexe binary.
//  * opts is optional (may be nil to take all defaults)
//  * deadlineEvtCh is optional. When supplied, it must be the cleanup channel
//    returned by calling `lucictx.AdjustDeadline` in the outer layer. While
//    the luciexe subprocess is running, if
//      - `InterruptEvent` or `TimeoutEvent` is received, the subprocess will
//         be terminated.
//      - `ClosureEvent` is received, the subprocess will be killed.
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
func Start(ctx context.Context, luciexeArgs []string, input *bbpb.Build, opts *Options, deadlineEvtCh <-chan lucictx.DeadlineEvent) (*Subprocess, error) {
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
		err.Assign(0, errors.Annotate(launchOpts.stdout.Close(), "closing stdout").Err())
		err.Assign(1, errors.Annotate(launchOpts.stderr.Close(), "closing stderr").Err())
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

	if deadlineEvtCh != nil {
		go func() {
			select {
			case <-closeChannels:
				// luciexe subprocess exits normally
			case firstEvt := <-deadlineEvtCh:
				s.firstDeadlineEvent.Store(firstEvt)
				logging.Warningf(ctx, "Received %s", firstEvt)
				terminateSent := false
				evt := firstEvt
				for {
					switch evt {
					case lucictx.InterruptEvent, lucictx.TimeoutEvent:
						if !terminateSent {
							if err := s.Terminate(); err != nil {
								logging.Errorf(ctx, "failed to terminate luciexe subprocess, reason: %s", err)
								break
							}
							terminateSent = true
						}
					case lucictx.ClosureEvent:
						if err := s.Kill(); err != nil {
							logging.Errorf(ctx, "failed to kill luciexe subprocess, reason: %s", err)
						}
						// Try kill only once even if it fails. By the time a ClosureEvent
						// event is received, if `lucictx.AdjustDeadline()` is applied
						// correctly, ctx should be cancelled already, thus the subprocess
						// should have been killed already as it is launched via
						// `exec.CommandContext`.
						return
					default:
						panic(fmt.Sprintf("impossible DeadlineEvent %s", evt))
					}
					evt = <-deadlineEvtCh
				}
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
			setTimeout := s.firstDeadlineEvent.Load() == lucictx.TimeoutEvent
			if !setTimeout {
				setTimeout, _ = lucictx.CheckDeadlines(s.ctx)
			}
			if setTimeout {
				proto.Merge(s.build, &bbpb.Build{
					StatusDetails: &bbpb.StatusDetails{
						Timeout: &bbpb.StatusDetails_Timeout{},
					},
				})
			}
		}()

		defer func() {
			if evt := s.firstDeadlineEvent.Load(); evt != nil {
				var errMsg string
				switch {
				case evt == lucictx.InterruptEvent:
					errMsg = "luciexe process is interrupted"
				case evt == lucictx.TimeoutEvent:
					errMsg = "luciexe process timed out"
				case evt == lucictx.ClosureEvent:
					errMsg = "luciexe process's context is cancelled"
				}
				if s.err != nil {
					s.err = errors.Annotate(s.err, "%s; luciexe error:", errMsg).Err()
				} else {
					s.err = errors.New(errMsg)
				}
			}
		}()
		// No matter what, we want to close stdout/stderr; if none of the other
		// return values have set `err`, it will be set to the result of closing
		// stdout/stderr.
		defer func() {
			close(s.closeChannels)
			if closeErr := <-s.allClosed; s.err == nil {
				s.err = closeErr
			}
		}()

		if s.err = s.cmd.Wait(); s.err != nil {
			s.err = errors.Annotate(s.err, "waiting for luciexe").Err()
			return
		}
		s.build, s.err = luciexe.ReadBuildFile(s.collectPath)
	})
	return s.build, s.err
}

// fieldsToClear are a set of fields that MUST be cleared in the initial build
// to luciexe.
var fieldsToClear = stringset.NewFromSlice(
	"end_time",
	"status_details",
	"summary_markdown",
	"steps",
	"tags",
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
	ib.CreateTime = timestamppb.New(clock.Now(ctx))
	ib.StartTime = timestamppb.New(clock.Now(ctx))
	ib.Status = bbpb.Status_STARTED
	return ib
}
