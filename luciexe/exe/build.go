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
	"google.golang.org/protobuf/proto"
)

type build struct {
	stepNamesMu sync.Mutex
	stepNames   map[string]int

	// allMu is held in R mode while any Modify methods are used, and in W mode
	// while the Build is being sent.
	allMu sync.RWMutex

	version     int64 // incremented for each RUnlock of allMu
	triggerSink chan struct{}

	// Each of these locks protects a single portion of `build`. allMu.RLock is
	// held before acquiring these.
	outMu    sync.Mutex
	statusMu sync.Mutex
	stepsMu  sync.Mutex

	build *bbpb.Build
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
	b.stepNamesMu.Lock()
	defer b.stepNamesMu.Unlock()

	curNS := getNS(ctx)
	var newName string

	curNum := b.stepNames[name]
	b.stepNames[name]++
	if curNum == 0 {
		newName = name
	} else {
		newName = fmt.Sprintf("%s (%d)", name, curNum+1)
	}

	newNS := fmt.Sprintf("%s|%s", curNS, newName)
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
	state := &build{
		triggerSink: make(chan struct{}, 1),
		build:       proto.Clone(initial).(*bbpb.Build),
	}

	waitCh := make(chan struct{})

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

					case <-state.triggerSink:
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

	return setBuild(ctx, state), waitCh
}
