// Copyright 2019 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain b copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package buildmerge implements the build.proto tracking and merging logic for
// luciexe host applications.
//
// You probably want to use `go.chromium.org/luci/luciexe/host` instead.
//
// This package is separate from luciexe/host to avoid unnecessary entaglement
// with butler/logdog; All the logic here is implemented to avoid:
//
//   * interacting with the environment
//   * interacting with butler/logdog (except by implementing callbacks for
//     those, but only acting on simple datastructures/proto messages)
//   * handling errors in any 'brutal' ways (all errors in this package are
//     handled by reporting them directly in the data structures that this
//     package manipulates).
//
// This is done to simplify testing (as much as it can be) by concentrating all
// the environment stuff into luciexe/host, and all the 'pure' functional stuff
// here (search "imperative shell, functional core").
package buildmerge

import (
	"context"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler"
	"go.chromium.org/luci/logdog/client/butler/bundler"
	"go.chromium.org/luci/luciexe"
	"golang.org/x/time/rate"
)

// CalcURLFn is a stateless function which can calculate the absolute url and
// viewUrl from a given logdog namespace (with trailing slash) and streamName.
type CalcURLFn func(namespaceSlash, streamName string) (url, viewUrl string)

// Agent holds all the logic around merging build.proto streams.
type Agent struct {
	// used to cancel in-progress sendMerge calls.
	parentCtx context.Context

	// MergedBuildC is the channel of all the merged builds.
	MergedBuildC <-chan *bbpb.Build
	// mergedBuildC is the send side of MergedBuildC
	mergedBuildC chan<- *bbpb.Build

	// userNamespaceSlash is the logdog namespace (with a trailing slash) which
	// we'll use to determine if a new stream is potentially monitored, or not.
	userNamespaceSlash string

	// userRootURL is the full url ('logdog://.../stream/build.proto') of the
	// user's "root" build.proto stream (i.e. the one emitted by the top level
	// luciexe implementation.
	//
	// This is used as a key to start the merge process.
	userRootURL string
	baseBuild   *bbpb.Build

	// statesMu covers `states`. It must be held when reading or writing to
	// `states`, but doesn't need to be held while interacting with an individual
	// *buildState obtained from the map.
	statesMu sync.RWMutex

	// states maps a stream URL (i.e. `logdog://.../stream/build.proto`) to the
	// state tracker for that stream.
	states map[string]*buildStateTracker

	// mergeCh is used in production mode to send pings via informNewData
	mergeCh dispatcher.Channel

	// informNewData is used to 'ping' mergeCh; it's overwritten in tests.
	informNewData func()

	doneMu sync.Mutex
	done   bool

	doneCloser func()
	doneCtx    context.Context

	// calculateURLs is a function which can convert a logdog namespace and
	// streamname into both the full 'Url' and 'ViewUrl' values for a Log message.
	// This is used by the buildMerger itself when deriving keys for the `states`
	// map, as well as for individual buildState objects to adjust their build's
	// logs' URLs.
	calculateURLs CalcURLFn
}

// New returns a new Agent.
//
// Args:
//   * ctx - used for logging and clock
//   * userNamespaceSlash - The logdog namespace (with a trailing slash) under
//     which we should monitor streams.
//   * base - The "model" Build message that all generated builds should start
//     with. All build proto streams will be merged onto a copy of this message.
//   * calculateURLs - A function to calculate Log.Url and Log.ViewUrl values.
//     Should be a pure function.
//
// The following fields will be merged into `base` from the user controlled
// build.proto stream(s):
//
//   Steps
//   SummaryMarkdown
//   Status
//   StatusDetails
//   UpdateTime
//   Tags
//   EndTime
//   Output
func New(ctx context.Context, userNamespaceSlash string, base *bbpb.Build, calculateURLs CalcURLFn) *Agent {
	ch := make(chan *bbpb.Build)
	userRootURL, _ := calculateURLs(userNamespaceSlash, luciexe.BuildProtoStreamSuffix)

	ret := &Agent{
		parentCtx: ctx,

		MergedBuildC: ch,

		mergedBuildC:       ch,
		states:             map[string]*buildStateTracker{},
		calculateURLs:      calculateURLs,
		userNamespaceSlash: userNamespaceSlash,
		userRootURL:        userRootURL,
		baseBuild:          proto.Clone(base).(*bbpb.Build),
	}
	var err error
	ret.mergeCh, err = dispatcher.NewChannel(ctx, &dispatcher.Options{
		QPSLimit: rate.NewLimiter(rate.Inf, 1),
		Buffer: buffer.Options{
			MaxLeases:    1,
			BatchSize:    1,
			FullBehavior: &buffer.DropOldestBatch{},
		},
		DrainedFn: ret.finalize,
	}, ret.sendMerge)
	if err != nil {
		panic(err) // creating dispatcher with static config should never fail
	}
	ret.informNewData = func() {
		ret.mergeCh.C <- nil // content doesn't matter
	}
	ret.doneCtx, ret.doneCloser = context.WithCancel(ctx)

	return ret
}

// Attach should be called once to attach this to a Butler.
//
// This must be done before the butler recieves any build.proto streams.
func (a *Agent) Attach(b *butler.Butler) {
	b.AddStreamRegistrationCallback(a.onNewStream, true)
}

func (a *Agent) onNewStream(desc *logpb.LogStreamDescriptor) bundler.StreamChunkCallback {
	if !a.collectingData() {
		return nil
	}
	if !strings.HasPrefix(desc.Name, a.userNamespaceSlash) || path.Base(desc.Name) != luciexe.BuildProtoStreamSuffix {
		return nil
	}

	var err error
	if desc.ContentType != luciexe.BuildProtoContentType {
		err = errors.Reason("stream %q has content type %q, expected %q", desc.Name, desc.ContentType, luciexe.BuildProtoContentType).Err()
	} else if desc.StreamType != logpb.StreamType_DATAGRAM {
		err = errors.Reason("stream %q has type %q, expected %q", desc.Name, desc.StreamType, logpb.StreamType_DATAGRAM).Err()
	}

	url, _ := a.calculateURLs("", desc.Name)
	bState := newBuildStateTracker(a.doneCtx, a, path.Dir(desc.Name)+"/", err)

	a.statesMu.Lock()
	defer a.statesMu.Unlock()
	a.states[url] = bState
	return bState.handleNewData
}

// Close causes the Agent to stop collecting data, emit a final merged build,
// and then shut down all internal routines.
func (a *Agent) Close() {
	fmt.Println("CLOSING AGENT")

	a.doneMu.Lock()
	alreadyClosed := a.done
	a.done = true // stops accepting new trackers
	a.doneMu.Unlock()

	if alreadyClosed {
		return
	}

	a.doneCloser() // stops all current state trackers

	// wait for all states' final work items
	for _, t := range a.snapStates() {
		t.getFinal()
	}

	// tells our merge Channel to process all the current (now-final) states for
	// one last build.
	a.informNewData()

	// shut down the mergeCh so it will no longer accept new informNewData calls.
	a.mergeCh.Close()
}

func (a *Agent) snapStates() map[string]*buildStateTracker {
	a.statesMu.RLock()
	trackers := make(map[string]*buildStateTracker, len(a.states))
	for k, v := range a.states {
		trackers[k] = v
	}
	a.statesMu.RUnlock()
	return trackers
}

func (a *Agent) sendMerge(_ *buffer.Batch) error {
	trackers := a.snapStates()

	states := make(map[string]*buildState, len(trackers))
	stepCount := 0
	for k, v := range trackers {
		state := v.getLatest()
		if !state.invalid {
			stepCount += len(state.build.GetSteps())
		}
		states[k] = state
	}

	base := *a.baseBuild // shallow copy; we only assign to top level fields
	base.Steps = nil
	if stepCount > 0 {
		base.Steps = make([]*bbpb.Step, 0, stepCount)
	}

	var insertSteps func(stepNS []string, streamURL string) *bbpb.Build
	insertSteps = func(stepNS []string, streamURL string) *bbpb.Build {
		state := states[streamURL]
		if state != nil && !state.invalid {
			for _, step := range state.build.Steps {
				isMergeStep := len(step.Logs) == 1 && step.Logs[0].Name == luciexe.BuildProtoLogName
				if isMergeStep || len(stepNS) > 0 {
					stepVal := *step
					step = &stepVal
				}
				baseName := step.Name
				if len(stepNS) > 0 {
					step.Name = strings.Join(append(stepNS, step.Name), "|")
				}

				base.Steps = append(base.Steps, step)

				if isMergeStep {
					subBuild := insertSteps(append(stepNS, baseName), step.Logs[0].Url)
					updateStepFromBuild(step, subBuild)
				}
			}
		}
		if state == nil {
			return nil
		}
		return state.build
	}
	updateBaseFromUserBuild(&base, insertSteps(nil, a.userRootURL))

	select {
	case a.mergedBuildC <- &base:
	case <-a.parentCtx.Done():
		a.Close()
	}

	return nil
}

func (a *Agent) finalize() {
	close(a.mergedBuildC)
}

func (a *Agent) collectingData() bool {
	a.doneMu.Lock()
	defer a.doneMu.Unlock()
	return !a.done
}

// Used for minting protobuf timestamps for buildStateTrackers
func (a *Agent) clockNow() *timestamp.Timestamp {
	ret, err := ptypes.TimestampProto(clock.Now(a.parentCtx))
	if err != nil {
		panic(err)
	}
	return ret
}
