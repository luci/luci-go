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

package build

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
)

// State tracks the state of the current buildbucket Build in the context.
//
// This object is provided to the `MainFn` at the start of the process, but it
// is also partially available through the context via functions like `WithStep`
// and `ModifyProperties`.
type State struct {
	// stepNames contains disambiguation state for duplicate names; Each FULL
	// name, as requested by the user, is the key, and the number of times that
	// full name was requested is the value.
	stepNamesMu sync.Mutex
	stepNames   map[string]int

	// modMu is held in Shared mode while the user is modifying `build`, and held
	// in Exclusive mode while the Build is being cloned for sending.
	modMu sync.RWMutex

	detached bool
	sender   dispatcher.Channel

	// version is incremented atomically by all user modification actions before
	// releasing Shared mode on modMu. It's observed by the sender while modMu is
	// under Exclusive mode.
	version int64

	outputMu sync.Mutex // protects `View` fields
	propsMu  sync.Mutex // protects `build.Output.Properties`
	stepsMu  sync.Mutex // protects `build.steps` (only for adding steps)

	// build is the current Build object state.
	build *bbpb.Build

	// initial is an immutable copy of the original Build.
	initial *bbpb.Build

	// sendFn is the user-provided function which should 'sink' updates to
	// `build`. The sender loop in SinkBuildUpdates will invoke this function with
	// clones of `build` when there are new versions of `build` to send.
	sendFn func(*bbpb.Build) error
}

// InitialBuild returns a copy of the input build that this Build was created
// with (typically, this is the raw Build which was scanned from stdin when the
// program started).
func (s *State) InitialBuild() *bbpb.Build {
	return proto.Clone(s.initial).(*bbpb.Build)
}

func (s *State) signalNewVersionLocked() {
	if s.sender.C != nil {
		s.sender.C <- atomic.AddInt64(&s.version, 1)
	}
}

// modLock is used to synchronize all access to the underlying Build proto.
//
// Anything which modifies any portion of the Build.build object must do so
// within the modLock callback. When the callback returns (regardless of error)
// b.sender will be notified that there's a new version of Build to send.
//
// Multiple goroutines can be inside modLock at the same time; It's up to the
// individual callers of modLock to ensure proper synchronization over the bits
// of b.build which they provide access to (usually via a second finer-grained
// Mutex). When locking bits of b.build, modLock should always be the first
// acquired and the last released.
//
// See WithState for the other half of the coordination via b.modMu.
func (s *State) modLock(cb func() error) error {
	s.modMu.RLock()
	defer s.modMu.RUnlock()
	if s.detached {
		return ErrBuildDetached
	}
	defer s.signalNewVersionLocked()
	return cb()
}

// enterDisambiguatedNamespace updates ctx with the new step namespace implied
// by `name`.
//
// If `name` would lead to a duplicate namespace, this will append ` (N)` to
// `name` to disambiguate it.
//
// This will continue to increment N until it finds an unused namespace.
//
// Panics if `name` contains "|" (see WithStep which checks for this).
func (s *State) enterDisambiguatedNamespace(ctx context.Context, name string) (string, context.Context) {
	if strings.Contains(name, "|") {
		panic(errors.Reason(
			"precondition violated: enterDisambiguatedNamespace called with `|` in `name`: %q",
			name).Err())
	}

	baseName := name
	if curNS := getNS(ctx); curNS != "" {
		baseName = fmt.Sprintf("%s|%s", curNS, name)
	}

	s.stepNamesMu.Lock()
	defer s.stepNamesMu.Unlock()

	var stepNS string
	curNum := s.stepNames[baseName]

	stepNS = baseName
	for s.stepNames[stepNS] != 0 {
		curNum++
		stepNS = fmt.Sprintf("%s (%d)", baseName, curNum)
	}

	// reset stepNames[baseName] to the deduplication number we just figured out;
	// No values less than curNum will work for the rest of the program.
	s.stepNames[baseName] = curNum

	// Claim the final computed name, even if it was the result of deduplication
	// as well...  Otherwise users could manually conflict with one of our
	// deduplicated names.
	s.stepNames[stepNS]++

	return stepNS, withNS(ctx, stepNS)
}

// View is a struct containing the modifiable portions of a Build.
//
// This specifically EXCLUDES Output.Properties, Output.Logs and Status*.
// Use ModifyProperties, the various Log* methods and the error returned from
// Run for these.
//
// Used by Build.Modify.
type View struct {
	SummaryMarkdown string
	Critical        bbpb.Trinary
	GitilesCommit   *bbpb.GitilesCommit
}

// Modify runs your callback with process-exclusive access to modify
// the current build's various 'status' fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// You must not retain pointers to anything you assign to the View
// message after the return of `cb`, or this could lead to tricky race
// conditions.
//
// Passes through `cb`'s result.
func (s *State) Modify(cb func(*View) error) error {
	return s.modLock(func() error {
		s.outputMu.Lock()
		defer s.outputMu.Unlock()
		bv := &View{
			SummaryMarkdown: s.build.GetSummaryMarkdown(),
			Critical:        s.build.GetCritical(),
			GitilesCommit:   s.build.GetOutput().GetGitilesCommit(),
		}

		err := cb(bv)
		s.build.SummaryMarkdown = bv.SummaryMarkdown
		s.build.Critical = bv.Critical
		s.build.Output.GitilesCommit = bv.GitilesCommit
		return err
	})
}
