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
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

type buildState struct {
	// build holds the most recently processed Build state
	build *bbpb.Build

	// closed is set to true when the build state is terminated and will recieve
	// no more updates.
	closed bool

	// invalid is set to true when the interior structure (i.e. Steps) of latest
	// contains invalid data and shouldn't be inspected.
	invalid bool
}

// buildStateTracker manages the state of a single build.proto datagram stream.
type buildStateTracker struct {
	merger *agent

	shouldCloseMu sync.Mutex
	shouldClose   bool

	work dispatcher.Channel

	// latestState holds *buildState
	latestState atomic.Value
}

func (bs *buildStateTracker) processClosure(state *buildState) {
	state.closed = true
	if state.build == nil {
		state.build = &bbpb.Build{
			SummaryMarkdown: "Never recieved any build data.",
			Status:          bbpb.Status_INFRA_FAILURE,
		}
	} else {
		processFinalBuild(bs.merger.clkNow(), state.build)
	}
}

func (bs *buildStateTracker) processData(state *buildState, data []byte, ns string) {
	now := bs.merger.clkNow()
	var parsedBuild *bbpb.Build
	err := func() error {
		build := &bbpb.Build{}
		if err := proto.Unmarshal(data, build); err != nil {
			return errors.Annotate(err, "parsing Build").Err()
		}
		parsedBuild = build

		for _, step := range parsedBuild.Steps {
			for _, log := range step.Logs {
				if err := types.StreamName(log.Url).Validate(); err != nil {
					step.Status = bbpb.Status_INFRA_FAILURE
					step.SummaryMarkdown += fmt.Sprintf("bad log url: %q", log.Url)
					return errors.Annotate(
						err, "step[%q].logs[%q].Url = %q", step.Name, log.Name, log.Url).Err()
				}

				log.Url, log.ViewUrl = bs.merger.calculateURLs(ns, log.Url)
			}
		}
		return nil
	}()
	if err != nil {
		if parsedBuild == nil {
			if state.build == nil {
				parsedBuild = &bbpb.Build{}
			} else {
				// make a shallow copy of the latest build
				buildVal := *state.build
				parsedBuild = &buildVal
			}
		}
		setErrorOnBuild(parsedBuild, err)
		processFinalBuild(now, parsedBuild)
		state.closed = true
		state.invalid = true
	}
	parsedBuild.UpdateTime = now

	state.build = parsedBuild
}

// newBuildState produces a new buildStateTracker tracker in the given logdog
// namespace.
//
// if `err` is provided, the buildStateTracker tracker is created in an errored
// (closed) state where GetLatest always returns a Build in the INFRA_FAILURE
// state with `err` reflected in the builds SummaryMarkdown.
func newBuildState(ctx context.Context, merger *agent, namespace string, err error) *buildStateTracker {
	if err != nil {
		ret := &buildStateTracker{}
		state := &buildState{build: &bbpb.Build{}, closed: true}
		setErrorOnBuild(state.build, err)
		ret.latestState.Store(state)
		return ret
	}

	ret := &buildStateTracker{merger: merger}

	workCh, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		Buffer: buffer.Options{
			MaxLeases:    1,
			BatchSize:    1,
			FullBehavior: &buffer.DropOldestBatch{},
		},
	}, func(data *buffer.Batch) error {
		state := *ret.GetLatest()
		// already closed
		if state.closed {
			return nil
		}

		ret.shouldCloseMu.Lock()
		shouldClose := ret.shouldClose
		ret.shouldCloseMu.Unlock()

		if shouldClose {
			// need to close
			ret.processClosure(&state)
		} else {
			// got data
			dg := data.Data[0].([]byte)
			ret.processData(&state, dg, namespace)
		}

		ret.latestState.Store(&state)
		merger.informNewData()
		return nil
	})
	if err != nil {
		panic(err)
	}

	ret.work = workCh
	ret.latestState.Store(&buildState{
		build: &bbpb.Build{
			SummaryMarkdown: "build.proto not found",
			Status:          bbpb.Status_SCHEDULED,
		},
	})
	return ret
}

// GetLatest returns the current state of the Build, as well as a boolean
// indicating that the Build's internal structure is safe to evaluate for merge
// steps.
//
// This always returns a non-nil Build to make the calling code simpler.
func (bs *buildStateTracker) GetLatest(waitForClose bool) *buildState {
	bs.shouldCloseMu.Lock()
	shouldClose := bs.shouldClose
	bs.shouldCloseMu.Unlock()
	if waitForClose && shouldClose {
		<-bs.work.DrainC
	}
	return bs.latestState.Load().(*buildState)
}

// StepCount is a convenience function to return the number of steps in the
// current Build state.
func (bs *buildStateTracker) StepCount(waitForClose bool) (ret int) {
	return len(bs.GetLatest(waitForClose).build.GetSteps())
}

// This implements the bundler.StreamChunkCallback callback function.
//
// Each call to `parse` expects `entry` to have a complete (non-Partial)
// datagram containing a single Build message. The message will be parsed and
// fixed up (e.g. fixing Log Url/ViewUrl), and become this buildStateTracker's new
// state.
//
// This method does not block, but calling GetLatest(true) after calling parse
// will
func (bs *buildStateTracker) parse(entry *logpb.LogEntry) {
	bs.shouldCloseMu.Lock()
	defer bs.shouldCloseMu.Unlock()

	if bs.shouldClose || !bs.merger.collectingData() {
		return // closed or closing
	}

	if entry == nil {
		bs.shouldClose = true
		bs.work.C <- nil
		bs.work.Close()
		return
	}

	bs.work.C <- entry.GetDatagram().Data
}
