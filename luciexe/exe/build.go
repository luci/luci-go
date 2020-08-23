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
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/time/rate"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldtypes "go.chromium.org/luci/logdog/common/types"
)

// Build tracks the state of the current buildbucket Build in this exe.
//
// This object is provided to the `MainFn` at the start of the process, but it
// is also partially available through the context via functions like `WithStep`
// and `ModifyProperties`.
type Build struct {
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

	logsMu   sync.Mutex // protects `build.Output.Logs`
	outputMu sync.Mutex // protects `BuildView` fields
	propsMu  sync.Mutex // protects `build.Output.Properties`
	stepsMu  sync.Mutex // protects `build.steps` (only for adding steps)

	// build is the current Build object state.
	build *bbpb.Build

	// initial is an immutable copy of the original Build.
	initial *bbpb.Build

	// Logdog client to sink log data to.
	ldClient *streamclient.Client

	// sinkFn is the user-provided function which should 'sink' updates to
	// `build`. The sender loop in SinkBuildUpdates will invoke this function with
	// clones of `build` when there are new versions of `build` to send.
	sinkFn func(*bbpb.Build)
}

// ldPrep adds a new Log to this Build's Output.Logs and returns a the
// build-unique logdog streamname allocated for this log.
func (b *Build) ldPrep(name string) (ldtypes.StreamName, error) {
	return addUniqueLog(-1, name, func(cb func(*[]*bbpb.Log) error) error {
		return b.modLock(func() error {
			b.logsMu.Lock()
			defer b.logsMu.Unlock()
			return cb(&b.build.Output.Logs)
		})
	})
}

// Log attaches a new text-mode log to the build with the given name.
//
// Caller must close the log when they're done with it.
func (b *Build) Log(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := b.ldPrep(name)
	if err != nil {
		return nil, err
	}
	return b.ldClient.NewTextStream(ctx, fullName, opts...)
}

// LogFile is a convenience method to attach a new text-mode log to the build
// with the given name and copy the contents of the file at `path` into it.
func (b *Build) LogFile(ctx context.Context, name string, path string, opts ...streamclient.Option) error {
	out, err := b.Log(ctx, name, opts...)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = io.Copy(out, in)
	return err
}

// LogBinary attaches a new binary-mode log to the build with the given name.
//
// Caller must close the log when they're done with it.
func (b *Build) LogBinary(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := b.ldPrep(name)
	if err != nil {
		return nil, err
	}
	return b.ldClient.NewBinaryStream(ctx, fullName, opts...)
}

// LogDatagram attaches a new datagram-mode log to the build with the given
// name.
//
// Caller must close the log when they're done with it.
func (b *Build) LogDatagram(ctx context.Context, name string, opts ...streamclient.Option) (streamclient.DatagramStream, error) {
	fullName, err := b.ldPrep(name)
	if err != nil {
		return nil, err
	}
	return b.ldClient.NewDatagramStream(ctx, fullName, opts...)
}

// InitialBuild returns a copy of the input build that this Build was created
// with (typically, this is the raw Build which was scanned from stdin when the
// program started).
func (b *Build) InitialBuild() *bbpb.Build {
	return proto.Clone(b.initial).(*bbpb.Build)
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
// See SinkBuildUpdates and Detach for the other half of the coordination
// via b.modMu.
func (b *Build) modLock(cb func() error) error {
	b.modMu.RLock()
	defer b.modMu.RUnlock()
	if b.detached {
		return ErrBuildDetached
	}
	if b.sender.C != nil {
		defer func() {
			b.sender.C <- atomic.AddInt64(&b.version, 1)
		}()
	}
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
func (b *Build) enterDisambiguatedNamespace(ctx context.Context, name string) (string, context.Context) {
	if strings.Contains(name, "|") {
		panic(errors.Reason(
			"precondition violated: enterDisambiguatedNamespace called with `|` in `name`: %q",
			name).Err())
	}

	baseName := name
	if curNS := getNS(ctx); curNS != "" {
		baseName = fmt.Sprintf("%s|%s", curNS, name)
	}

	b.stepNamesMu.Lock()
	defer b.stepNamesMu.Unlock()

	var stepNS string
	curNum := b.stepNames[baseName]

	stepNS = baseName
	for b.stepNames[stepNS] != 0 {
		curNum++
		stepNS = fmt.Sprintf("%s (%d)", baseName, curNum)
	}

	// reset stepNames[baseName] to the deduplication number we just figured out;
	// No values less than curNum will work for the rest of the program.
	b.stepNames[baseName] = curNum

	// Claim the final computed name, even if it was the result of deduplication
	// as well...  Otherwise users could manually conflict with one of our
	// deduplicated names.
	b.stepNames[stepNS]++

	return stepNS, withNS(ctx, stepNS)
}

// SinkBuildUpdates installs a `sinkFn` function into the context.
//
// `sinkFn` will be called with copies of the build, as updated by methods
// in this package (i.e. Build.Modify, Step.Modify, ModifyProperties, WithStep,
// etc.)
//
// The Run() function will already have a sinkFn installed which is wired up to
// logdog; However, you can use `SinkBuildUpdates` to run `luciexe` code in
// other contexts (e.g. you could run the code without any luciexe host
// process for local tests or command-line applications).
//
// `sinkFn` is called when modifications to the build happen, no faster than the
// slower of `lim` and the execution speed of your `sinkFn` function.
// Invocations of `sinkFn` are cumulative, meaning that if `lim` is 1Hz, and you
// have 100 updates per second, `sinkFn` will be called a maximum of once per
// second, with a build containing all 100 updates.
//
// If you intend to ignore all build updates, pass `nil` for sinkFn; All
// modifications to the build will still apply, but they won't be 'sent'
// anywhere. You can use `Detach` at the end of your program to retrieve the
// final state of the build. If you also want to ignore all log data, pass `nil`
// for ldClient (this is equivalent to passing streamclient.New("null", "")).
//
// To close and drain the buildsink, call Detach().
func SinkBuildUpdates(ctx context.Context, initial *bbpb.Build, ldClient *streamclient.Client, lim rate.Limit, sinkFn func(*bbpb.Build)) (nc context.Context, build *Build) {
	if ldClient == nil {
		var err error
		ldClient, err = streamclient.New("null", "")
		if err != nil {
			panic(err)
		}
	}

	build = &Build{
		build:     proto.Clone(initial).(*bbpb.Build),
		initial:   proto.Clone(initial).(*bbpb.Build),
		ldClient:  ldClient,
		stepNames: map[string]int{},
		sinkFn:    sinkFn,
	}

	if build.build.Output == nil {
		build.build.Output = &bbpb.Build_Output{}
	}

	if sinkFn == nil || lim <= 0 {
		drainC := make(chan struct{})
		build.sender = dispatcher.Channel{
			C:      nil, // modLock will skip all sends
			DrainC: drainC,
		}
		close(drainC)
	} else {
		var lastSent int64
		var err error
		build.sender, err = dispatcher.NewChannel(ctx, &dispatcher.Options{
			QPSLimit: rate.NewLimiter(lim, 1),
			Buffer: buffer.Options{
				MaxLeases: 1,
				BatchSize: 1,
				FullBehavior: &buffer.DropOldestBatch{
					MaxLiveItems: 1,
				},
			},
		}, func(data *buffer.Batch) error {
			if versionToSend := data.Data[0].(int64); versionToSend <= lastSent {
				return nil
			}

			build.modMu.Lock()
			toSend := proto.Clone(build.build).(*bbpb.Build)
			version := build.version
			build.modMu.Unlock()

			sinkFn(toSend)
			lastSent = version
			return nil
		})
		if err != nil {
			panic(err) // only if dispatcher.Options were invalid
		}
	}

	return setBuild(ctx, build), build
}

// Detach allows you to take full, manual, control over manipulating and sending
// the build.
//
// Calling this will close the sender associated with this build (if it's still
// running), and wait for it to drain (canceling `ctx` will skip waiting for the
// drain).
//
// After this, it returns the current build state, as well as a function bound
// to sending this build state.
//
// After calling Detach, the manipulation methods in this package will return
// ErrBuildDetached and have no other effect.
func (b *Build) Detach(ctx context.Context) (current *bbpb.Build, sendFn func()) {
	// Synchronize with any modLock operations to make sure they finish pushing
	// into sender, and then nil out C so that future modLock operations will not
	// attempt to push.
	b.modMu.Lock()
	if !b.detached {
		b.sender.Close()
		b.sender.C = nil
		b.detached = true
	}
	b.modMu.Unlock()

	// Wait for sender to drain now without lock (since it locks modMu in the
	// sender function)
	select {
	case <-b.sender.DrainC:
	case <-ctx.Done():
	}

	if b.sinkFn == nil {
		return b.build, nil
	}
	return b.build, func() { b.sinkFn(b.build) }
}

// BuildView is a struct containing the modifiable portions of a Build.
//
// This specifically EXCLUDES Output.Properties, Output.Logs and Status*.
// Use ModifyProperties, the various Log* methods and the error returned from
// Run for these.
//
// Used by Build.Modify.
type BuildView struct {
	SummaryMarkdown string
	Critical        bbpb.Trinary
	GitilesCommit   *bbpb.GitilesCommit
}

// Modify runs your callback with process-exclusive access to modify
// the current build's various 'status' fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// You must not retain pointers to anything you assign to the BuildView
// message after the return of `cb`, or this could lead to tricky race
// conditions.
//
// Passes through `cb`'s result.
func (b *Build) Modify(cb func(*BuildView) error) error {
	return b.modLock(func() error {
		b.outputMu.Lock()
		defer b.outputMu.Unlock()
		bv := &BuildView{
			SummaryMarkdown: b.build.GetSummaryMarkdown(),
			Critical:        b.build.GetCritical(),
			GitilesCommit:   b.build.GetOutput().GetGitilesCommit(),
		}

		err := cb(bv)
		b.build.SummaryMarkdown = bv.SummaryMarkdown
		b.build.Critical = bv.Critical
		b.build.Output.GitilesCommit = bv.GitilesCommit
		return err
	})
}
