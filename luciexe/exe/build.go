// Copyright 2020 The LUCI Authors.
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

package exe

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/proto"
)

type build struct {
	// stepNames contains disambiguation state for duplicate names; Each FULL
	// name, as requested by the user, is the key, and the number of times that
	// full name was requested is the value.
	stepNamesMu sync.Mutex
	stepNames   map[string]int

	// allMu is held in Shared mode while the user is modifying `build`, and held
	// in Exclusive mode while the Build is being cloned for sending.
	allMu sync.RWMutex

	// version is incremented atomically by all user modification actions before
	// releasing Shared mode on allMu. It's observed by the sender while allMu is
	// under Exclusive mode.
	version int64

	outMu    sync.Mutex // protects `build.Output`
	statusMu sync.Mutex // protects `BuildStatus` fields of `build`.
	stepsMu  sync.Mutex // protects `build.steps` (only for adding steps)

	// triggerSink is a depth-1 channel which is used as a select-able condition.
	// It is written to (non-blocking) after the user modification increments
	// `version`, and is read from by the sender to wake up and send a new
	// version.
	triggerSink chan<- struct{}

	// detached is an unbuffered channel which is closed by DetatchBuildSink in
	// order to retrieve `build` and `sinkFn`.
	detatchCh chan<- struct{}

	// waitCh is an unbuffered channel which is closed by the sender loop when it
	// quits.
	waitCh <-chan struct{}

	// build is the current Build object state.
	build *bbpb.Build

	// sinkFn is the user-provided function which should 'sink' updates to
	// `build`. The sender loop in SinkBuildUpdates will invoke this function with
	// clones of `build` when there are new versions of `build` to send.
	sinkFn func(*bbpb.Build)
}

func (b *build) modLock(cb func() error) error {
	b.allMu.RLock()
	defer b.allMu.RUnlock()
	defer func() {
		// atomic to increment version because multiple stacks can be in modLock at
		// the same time; However, this is atomically incremented while holding
		// allMu.RLock. In the sink thread, it will obtain allMu.Lock before
		// observing version.
		atomic.AddInt64(&b.version, 1)
		select {
		case b.triggerSink <- struct{}{}:
		default:
		}
	}()
	return cb()
}

func (b *build) snapshot(newerThan int64) (build *bbpb.Build, version int64) {
	b.allMu.Lock()
	defer b.allMu.Unlock()
	if version = b.version; version > newerThan {
		build = proto.Clone(b.build).(*bbpb.Build)
	}
	return
}

// enterDisambiguatedNamespace updates ctx with the new namespace implied by
// `name`.
//
// `name` must not contain "|"
func (b *build) enterDisambiguatedNamespace(ctx context.Context, name string) (string, context.Context) {
	baseName := name
	if curNS := getNS(ctx); curNS != "" {
		baseName = fmt.Sprintf("%s|%s", curNS, name)
	}

	b.stepNamesMu.Lock()
	curNum := b.stepNames[baseName]
	b.stepNames[baseName]++
	b.stepNamesMu.Unlock()

	var newNS string
	if curNum == 0 {
		newNS = name
	} else {
		newNS = fmt.Sprintf("%s (%d)", baseName, curNum+1)
	}

	return newNS, withNS(ctx, newNS)
}

// SinkBuildUpdates installs a `sink` function into the context.
//
// This function will be called with copies of the Build, as updated by methods
// in this package (i.e. ModifyBuildOutput, ModifyBuildStatus, WithStep, etc.)
//
// The Run() function will already have a sink installed which is wired up to
// logdog; However, you can use this function to run `luciexe` code in other
// contexts.
//
// Updates are sent when modifications to the build happen, and no faster than
// your `sink` function executes; If `sink` blocks for 5 minutes, and there are
// 100 updates in that time, sink will be called again with the cumulative
// update within those 5 minutes. This makes implementing rate control easy.
//
// If you intend to ignore all build updates, pass `nil` for sink.
//
// To close and flush the buildsink, cancel `ctx` and then read from the
// returned waiter channel.
func SinkBuildUpdates(ctx context.Context, initial *bbpb.Build, sink func(*bbpb.Build)) (nc context.Context, wait <-chan struct{}) {
	waitCh := make(chan struct{})
	detatchCh := make(chan struct{})
	triggerSink := make(chan struct{}, 1)

	state := &build{
		build:       proto.Clone(initial).(*bbpb.Build),
		sinkFn:      sink,
		triggerSink: triggerSink,
		detatchCh:   detatchCh,
		waitCh:      waitCh,
	}

	if sink != nil {
		go func() {
			defer close(waitCh)

			lastSent := int64(-1)
			drain := false

			for {
				if !drain {
					select {
					case <-ctx.Done():
						drain = true

					case <-triggerSink:
					case <-detatchCh:
						break
					}
				}

				toSend, version := state.snapshot(lastSent)

				if toSend != nil {
					sink(toSend)
					lastSent = version
				} else if drain {
					// draining and nothing new to send, so quit
					break
				}
			}
		}()
	} else {
		close(waitCh)
	}

	return setBuild(ctx, state), state.waitCh
}

// DetatchBuildSink allows you to take full, manual, control over manipulating
// and sending the current build.
//
// This function may only be called once per SinkBuildUpdates, panic'ing if
// called more than once. Once this function is called, none of the Modify*
// methods or WithStep will have any effect.
func DetatchBuildSink(ctx context.Context) (current *bbpb.Build, sinkFn func(*bbpb.Build)) {
	build := getBuild(ctx)

	defer func() {
		if err := recover(); err != nil {
			panic(errors.Reason("DetatchBuildSink called twice: %s", err).Err())
		}
	}()
	close(build.detatchCh)

	<-build.waitCh

	return build.build, build.sinkFn
}
