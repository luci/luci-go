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

package buildmerge

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// buildState represents the current state of a single build.proto stream.
type buildState struct {
	// build holds the most recently processed Build state. This message should be
	// treated as immutable (i.e. proto.Clone before modifying it).
	//
	// This may be `nil` until the first user-supplied build.proto is processed,
	// or until the buildStateTracker closes.
	build *bbpb.Build

	// buildReadOnly holds the most recently processed Build state to read.
	// This message should be treated as immutable (i.e. proto.Clone before modifying it).
	buildReadOnly *bbpb.Build

	// closed is set to true when the build state is terminated and will receive
	// no more user updates (but may still need to be finalized()).
	closed bool

	// final is set to true when the build state is closed and all final
	// processing has occurred on the build state.
	final bool

	// invalid is set to true when the interior structure (i.e. Steps) of latest
	// contains invalid data and shouldn't be inspected.
	invalid bool
}

// buildStateTracker manages the state of a single build.proto datagram stream.
type buildStateTracker struct {
	ctx context.Context

	// The Agent that this buildStateTracker belongs to. Used to access:
	//   * clockNow
	//   * calculateURLs
	//   * informNewData
	merger *Agent

	ldNamespace types.StreamName

	// True iff we should expect zlib-compressed datagrams.
	zlib bool

	// We use this mutex to synchronize closure and sending operations on the work
	// channel; `work` is configured, if it's running, to immediately accept any
	// items pushed to it, so it's safe to hold this while sending on work.C.
	workMu sync.Mutex

	// The work channel is configured to only keep the latest incoming datagram.
	// It's send function parses and interprets the Build message.
	// Errors are not reported to the dispatcher.Channel, but are instead recorded
	// in the parsed Build state.
	work       dispatcher.Channel[[]byte]
	workClosed bool // true if we've closed work.C, protected by workMu

	latestStateMu sync.Mutex
	latestState   *buildState
}

// updateState updates `state` with the Build.proto message inside the lock.
//
// If there's an error when generating the new build - i.e. when parsing `data`
// or an error in the decoded message's contents, `state.invalid` and
// `state.closed` will be set to true, and `state.build` will be updated with
// the error message.
func (t *buildStateTracker) updateState(newBuild *bbpb.Build, err error) {
	t.latestStateMu.Lock()
	defer t.latestStateMu.Unlock()
	state := *t.latestState
	oldBuild := state.build

	if state.closed {
		return
	}

	if err != nil {
		if newBuild == nil {
			if oldBuild == nil {
				newBuild = &bbpb.Build{}
			} else {
				newBuild = oldBuild
			}
		}
		setErrorOnBuild(newBuild, err)
		newBuild.UpdateTime = t.merger.clockNow()
		state.closed = true
		state.invalid = true
	}

	state.build = newBuild
	// Reset buildReadOnly since we have a new build state now.
	state.buildReadOnly = nil

	if state.closed {
		t.Close()
	}

	t.latestState = &state
}

// parseBuild parses `data` then returns the parsed Build.
func (t *buildStateTracker) parseBuild(data []byte) (*bbpb.Build, error) {
	if t.zlib {
		z, err := zlib.NewReader(bytes.NewBuffer(data))
		if err != nil {
			return nil, errors.Annotate(err, "constructing decompressor for Build").Err()
		}
		data, err = io.ReadAll(z)
		if err != nil {
			return nil, errors.Annotate(err, "decompressing Build").Err()
		}
	}

	parsedBuild := &bbpb.Build{}
	if err := proto.Unmarshal(data, parsedBuild); err != nil {
		return nil, errors.Annotate(err, "parsing Build").Err()
	}

	for _, step := range parsedBuild.Steps {
		if len(step.Logs) > 0 && step.Logs[0].Name == "$build.proto" {
			// convert incoming $build.proto logs to MergeBuild messages.
			// If the step has both, then just discard the $build.proto log.
			//
			// TODO(crbug.com/1310155): Remove this conversion after everything
			// emits MergeBuild messages natively.
			if step.MergeBuild == nil {
				step.MergeBuild = &bbpb.Step_MergeBuild{
					FromLogdogStream: step.Logs[0].Url,
				}
			}
			step.Logs = step.Logs[1:]
		}
		for _, log := range step.Logs {
			var err error
			log.Url, log.ViewUrl, err = absolutizeURLs(log.Url, log.ViewUrl, t.ldNamespace, t.merger.calculateURLs)
			if err != nil {
				step.Status = bbpb.Status_INFRA_FAILURE
				step.SummaryMarkdown += err.Error()
				return parsedBuild, errors.Annotate(err, "step[%q].logs[%q]", step.Name, log.Name).Err()
			}
		}
		if mb := step.GetMergeBuild(); mb != nil && mb.FromLogdogStream != "" {
			var err error
			mb.FromLogdogStream, _, err = absolutizeURLs(mb.FromLogdogStream, "", t.ldNamespace, t.merger.calculateURLs)
			if err != nil {
				step.Status = bbpb.Status_INFRA_FAILURE
				step.SummaryMarkdown += err.Error()
				return parsedBuild, errors.Annotate(err, "step[%q].merge_build.from_logdog_stream", step.Name).Err()
			}
		}
	}
	for _, log := range parsedBuild.GetOutput().GetLogs() {
		var err error
		log.Url, log.ViewUrl, err = absolutizeURLs(log.Url, log.ViewUrl, t.ldNamespace, t.merger.calculateURLs)
		if err != nil {
			return parsedBuild, errors.Annotate(err, "build.output.logs[%q]", log.Name).Err()
		}
	}
	parsedBuild.UpdateTime = t.merger.clockNow()
	return parsedBuild, nil
}

// newBuildStateTracker produces a new buildStateTracker in the given logdog
// namespace.
//
// `ctx` is used for cancellation/logging.
//
// `merger` is the Agent that this buildStateTracker belongs to. See the comment
// in buildStateTracker for its use of this.
//
// `namespace` is the logdog namespace under which this build.proto is being
// streamed from. e.g. if the updates to handleNewData are coming from a logdog
// stream "a/b/c/build.proto", then `namespace` here should be "a/b/c". This is
// used verbatim as the namespace argument to merger.calculateURLs.
//
// if `err` is provided, the buildStateTracker tracker is created in an errored
// (closed) state where getLatest always returns a fixed Build in the
// INFRA_FAILURE state with `err` reflected in the build's SummaryMarkdown
// field.
func newBuildStateTracker(ctx context.Context, merger *Agent, namespace types.StreamName, zlib bool, err error) *buildStateTracker {
	ret := &buildStateTracker{
		ctx:         ctx,
		merger:      merger,
		zlib:        zlib,
		ldNamespace: namespace.AsNamespace(),
		latestState: &buildState{},
	}

	if err != nil {
		ret.latestState.build = &bbpb.Build{}
		setErrorOnBuild(ret.latestState.build, err)
		ret.finalize()
		ret.Close()
	} else {
		ret.work, err = dispatcher.NewChannel[[]byte](ctx, &dispatcher.Options[[]byte]{
			Buffer: buffer.Options{
				MaxLeases:     1,
				BatchItemsMax: 1,
				FullBehavior:  &buffer.DropOldestBatch{},
			},
			DropFn:    dispatcher.DropFnQuiet[[]byte],
			DrainedFn: ret.finalize,
		}, ret.parseAndSend)
		if err != nil {
			panic(err) // creating dispatcher with static config should never fail
		}
		// Attach the cancelation of the context to the closure of work.C.
		go func() {
			select {
			case <-ctx.Done():
				ret.Close()
			case <-ret.work.DrainC:
				// already shut down w/o cancelation
			}
		}()
	}

	return ret
}

// finalized is called exactly once when either:
//
//   - newBuildStateTracker is called with err != nil
//   - buildStateTracker.work is fully shut down (this is installed as
//     dispatcher.Options.DrainedFn)
func (t *buildStateTracker) finalize() {
	t.latestStateMu.Lock()
	defer t.latestStateMu.Unlock()

	state := *t.latestState
	if state.final {
		panic("impossible; finalize called twice?")
	}

	state.closed = true
	state.final = true
	if state.build == nil {
		state.build = &bbpb.Build{
			SummaryMarkdown: "Never received any build data.",
			Status:          bbpb.Status_INFRA_FAILURE,
			Output: &bbpb.Build_Output{
				Status:          bbpb.Status_INFRA_FAILURE,
				SummaryMarkdown: "Never received any build data.",
			},
		}
	}
	processFinalBuild(t.merger.clockNow(), state.build)
	state.buildReadOnly = nil
	t.latestState = &state
	t.merger.informNewData()
}

func (t *buildStateTracker) parseAndSend(data *buffer.Batch[[]byte]) error {
	t.latestStateMu.Lock()
	state := *t.latestState
	t.latestStateMu.Unlock()

	// already closed
	if state.closed {
		return nil
	}

	newBuild, err := t.parseBuild(data.Data[0].Item)
	// may set state.closed on an error
	t.updateState(newBuild, err)

	t.merger.informNewData()
	return nil
}

// getLatestBuild returns the Build in the current state.
//
// It returns the internal read-only copy of the build to avoid the read/write race.
func (t *buildStateTracker) getLatestBuild() *bbpb.Build {
	t.latestStateMu.Lock()
	defer t.latestStateMu.Unlock()

	// Lazily clone the build to its read-only copy when needed.
	if t.latestState.buildReadOnly == nil {
		t.latestState.buildReadOnly = proto.Clone(t.latestState.build).(*bbpb.Build)
	}
	return t.latestState.buildReadOnly
}

// This implements the bundler.StreamChunkCallback callback function.
//
// Each call to `handleNewData` expects `entry` to have a complete (non-Partial)
// datagram containing a single Build message. The message will (eventually) be
// parsed and fixed up (e.g. fixing Log Url/ViewUrl), and become this
// buildStateTracker's new state.
//
// This method does not block; Data here is submitted to the buildStateTracker's
// internal worker, which processes state updates as quickly as it can, skipping
// state updates which are submitted too rapidly.
//
// This method has no effect if the buildStateTracker is 'closed'.
//
// When this is called with `nil` as an argument (when the attached logdog
// stream is closed), it will start the closure process on this
// buildStateTracker. The final build state can be obtained synchronously by
// calling GetFinal().
func (t *buildStateTracker) handleNewData(entry *logpb.LogEntry) {
	t.workMu.Lock()
	defer t.workMu.Unlock()

	if entry == nil {
		t.closeWorkLocked()
	} else if !t.workClosed {
		select {
		case t.work.C <- entry.GetDatagram().Data:
		case <-t.ctx.Done():
			t.closeWorkLocked()
		}
	}
}

func (t *buildStateTracker) closeWorkLocked() {
	if !t.workClosed {
		if t.work.C != nil {
			close(t.work.C)
		}
		t.workClosed = true
	}
}

func (t *buildStateTracker) Close() {
	t.workMu.Lock()
	defer t.workMu.Unlock()
	t.closeWorkLocked()
}

// Drain waits for the build state to finalize.
func (t *buildStateTracker) Drain() {
	if t.work.DrainC != nil {
		<-t.work.DrainC
	}
}
