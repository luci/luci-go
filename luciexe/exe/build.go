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
	"sync"
	"sync/atomic"

	structpb "github.com/golang/protobuf/ptypes/struct"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldtypes "go.chromium.org/luci/logdog/common/types"
)

type Build struct {
	// stepNames contains disambiguation state for duplicate names; Each FULL
	// name, as requested by the user, is the key, and the number of times that
	// full name was requested is the value.
	stepNamesMu sync.Mutex
	stepNames   map[string]int

	// modMu is held in Shared mode while the user is modifying `build`, and held
	// in Exclusive mode while the Build is being cloned for sending.
	modMu sync.RWMutex

	// version is incremented atomically by all user modification actions before
	// releasing Shared mode on modMu. It's observed by the sender while modMu is
	// under Exclusive mode.
	version int64

	logsMu   sync.Mutex // protects `build.Output.Logs`
	outputMu sync.Mutex // protects `BuildView` fields
	propsMu  sync.Mutex // protects `build.Output.Properties`
	stepsMu  sync.Mutex // protects `build.steps` (only for adding steps)

	// triggerSink is a depth-1 channel which is used as a select-able condition.
	// It is written to (non-blocking) after the user modification increments
	// `version`, and is read from by the sender to wake up and send a new
	// version.
	triggerSink chan<- struct{}

	// detached is an unbuffered channel which is closed by DetachBuildSink in
	// order to retrieve `build` and `sinkFn`.
	detatchCh chan<- struct{}

	// waitCh is an unbuffered channel which is closed by the sender loop when it
	// quits.
	waitCh <-chan struct{}

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

func (b *Build) ldPrep(ctx context.Context, name string) (ldtypes.StreamName, error) {
	return ldPrep(ctx, []string{name}, func(cb func(*[]*bbpb.Log) error) error {
		return b.modLock(func() error {
			b.logsMu.Lock()
			defer b.logsMu.Unlock()
			return cb(&b.build.Output.Logs)
		})
	})
}

// Log attaches a new text-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (b *Build) Log(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := b.ldPrep(ctx, name)
	if err != nil {
		return nil, err
	}
	return b.ldClient.NewTextStream(ctx, fullName, opts...)
}

// LogFile is a convenience method to attach a new text-mode log to the step
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

// LogBinary attaches a new binary-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (b *Build) LogBinary(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := b.ldPrep(ctx, name)
	if err != nil {
		return nil, err
	}
	return b.ldClient.NewBinaryStream(ctx, fullName, opts...)
}

// LogDatagram attaches a new datagram-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (b *Build) LogDatagram(ctx context.Context, name string, opts ...streamclient.Option) (streamclient.DatagramStream, error) {
	fullName, err := b.ldPrep(ctx, name)
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

func (b *Build) modLock(cb func() error) error {
	b.modMu.RLock()
	defer b.modMu.RUnlock()
	defer func() {
		// atomic to increment version because multiple stacks can be in modLock at
		// the same time; However, this is atomically incremented while holding
		// modMu.RLock. In the sink thread, it will obtain modMu.Lock before
		// observing version.
		atomic.AddInt64(&b.version, 1)
		select {
		case b.triggerSink <- struct{}{}:
		default:
		}
	}()
	return cb()
}

func (b *Build) snapshot(newerThan int64) (build *bbpb.Build, version int64) {
	b.modMu.Lock()
	defer b.modMu.Unlock()
	if version = b.version; version > newerThan {
		build = proto.Clone(b.build).(*bbpb.Build)
	}
	return
}

// enterDisambiguatedNamespace updates ctx with the new namespace implied by
// `name`.
//
// `name` must not contain "|"
func (b *Build) enterDisambiguatedNamespace(ctx context.Context, name string) (string, context.Context) {
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
// in this package (i.e. Build.Modify, Step.Modify, ModifyProperties, WithStep,
// etc.)
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
func SinkBuildUpdates(ctx context.Context, initial *bbpb.Build, ldClient *streamclient.Client, sink func(*bbpb.Build)) (nc context.Context, build *Build) {
	waitCh := make(chan struct{})
	detatchCh := make(chan struct{})
	triggerSink := make(chan struct{}, 1)

	build = &Build{
		build:       proto.Clone(initial).(*bbpb.Build),
		initial:     proto.Clone(initial).(*bbpb.Build),
		ldClient:    ldClient,
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

				toSend, version := build.snapshot(lastSent)

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

	return setBuild(ctx, build), build
}

// Detach allows you to take full, manual, control over manipulating and sending
// the current build.
//
// This function may only be called once per SinkBuildUpdates, panic'ing if
// called more than once. Once this function is called, none of the Modify*
// methods or WithStep will have any effect.
func (b *Build) Detach() (current *bbpb.Build, sinkFn func(*bbpb.Build)) {
	defer func() {
		if err := recover(); err != nil {
			panic(errors.Reason("DetachBuildSink called twice: %s", err).Err())
		}
	}()
	close(b.detatchCh)

	<-b.waitCh

	return b.build, b.sinkFn
}

// Finalize sets a final status on the Build derived from the given error.
//
// panics if the Build already has a Status which is final.
func (b *Build) Finalize(err error) {
	if protoutil.IsEnded(b.build.Status) {
		panic(errors.New("Finalize called on finished Build"))
	}
	b.build.Status, b.build.StatusDetails = getErrorStatus(err)
}

// Wait waits until this Build has finished sending all updates.
//
// If Detach was called, this returns immediately (but someone else may still
// be using the send function).
func (b *Build) Wait() {
	<-b.waitCh
}

// BuildView is a struct containing the modifiable portions of a Build.
//
// This specifically EXCLUDES Output.Properties, Output.Logs and Status*.
// Use ModifyProperties, the various AddLog methods and the error returned from
// Run for those.
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
			SummaryMarkdown: b.build.SummaryMarkdown,
			Critical:        b.build.Critical,
			GitilesCommit:   b.build.Output.GitilesCommit,
		}

		err := cb(bv)
		b.build.SummaryMarkdown = bv.SummaryMarkdown
		b.build.Critical = bv.Critical
		b.build.Output.GitilesCommit = bv.GitilesCommit
		return err
	})
}

func mkEmptyStruct() *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{
		Fields: map[string]*structpb.Value{},
	}}}
}

// NamespaceProperties adjusts the 'root' property key given one or more
// namespace tokens.
//
// This is useful to allow some library code access to properties, but at
// a location of your chosing, and to be sure that that library can only see and
// manipulate data within its own scope.
//
// Namespaced properties are lazy; They will be populated in the build's output
// properties only when ModifyProperties is called.
//
// For example, given the properties:
//
//    {
//      "foo": {"bar": {"some": "things"}},
//      "numeric": 100,
//      "other": ...
//    }
//
// If you create a context with namespace=["foo", "bar"], then ModifyProperties
// using this contex will be able to see/mutate only {"some": "things"}.
//
// If you create a context with namespace=["numeric"], then ModifyProperties
// will overwrite the "numeric" value to {} when called, presenting the callback
// with an empty struct.
func NamespaceProperties(ctx context.Context, namespace ...string) context.Context {
	return addPropertyNS(ctx, namespace)
}

// ModifyProperties allows you to atomically read and write the output
// properties object within the current `NamespaceProperties` scope of the
// overall Build.
//
// See WriteProperties and ParseProperties, as well as the AsMap helper method
// on *Struct for assistance dealing with the Struct proto message.
func ModifyProperties(ctx context.Context, cb func(props *structpb.Struct) error) error {
	build := getBuild(ctx)
	propNS := getPropertyNS(ctx)

	return build.modLock(func() error {
		build.propsMu.Lock()
		defer build.propsMu.Unlock()

		node := build.build.Output.Properties
		if node == nil {
			build.build.Output.Properties = mkEmptyStruct().GetStructValue()
			node = build.build.Output.Properties
		}

		for i, tok := range propNS {
			newNode := node.Fields[tok].GetStructValue()
			if newNode == nil {
				if node.Fields[tok] != nil {
					logging.Debugf(ctx, "overwriting properties %q to Struct", propNS[:i])
				}
				node.Fields[tok] = mkEmptyStruct()
				newNode = node.Fields[tok].GetStructValue()
			}
		}

		return cb(node)
	})
}
