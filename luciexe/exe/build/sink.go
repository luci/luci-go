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
	"runtime/debug"

	"golang.org/x/time/rate"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

// Sink contains all options to create a "build sink" which will absorb updates
// to a buildbucket Build message via the Go context.
//
// Set initial options in a Sink struct and then Use it to get a context which
// absorbs updates to a Build.
type Sink struct {
	// InitialBuild is the basis build to use for sending.
	//
	// On usage of this Sink, it will be cloned.
	//
	// `nil` is equivalent to an empty Build.
	InitialBuild *bbpb.Build

	// LogdogClient is the client to use for all Log functions' underlying
	// io streams.
	//
	// `nil` is the equivalent of streamclient.New("null", "")
	LogdogClient *streamclient.Client

	// SendLimit is the maximum rate limit for invocations to SendFunc.
	//
	// If omitted, SendFunc will not be called (equivalent to SendFunc == nil).
	SendLimit rate.Limit

	// SendFunc is the function that this Sink will call when the build state
	// changes. It will be called at a maximum rate of SendLimit, and only one
	// SendFunc invocation will be active concurrently.
	//
	// It is called with a copy of the current build state, and does not block
	// updates to the build state.
	//
	// If the build state is updated more frequently than SendFunc operates,
	// intermediate updates will be 'rolled up' into the next invocation of
	// SendFunc.
	//
	// `nil` means that updates to the build state will not be sent anywhere.
	SendFunc func(*bbpb.Build) error
}

func (s Sink) makeSender(ctx context.Context, build *State) dispatcher.Channel {
	if s.SendFunc == nil || s.SendLimit <= 0 {
		drainC := make(chan struct{})
		sender := dispatcher.Channel{
			C:      nil, // modLock will skip all sends
			DrainC: drainC,
		}
		close(drainC)
		return sender
	}

	var lastSent int64
	var err error
	sender, err := dispatcher.NewChannel(ctx, &dispatcher.Options{
		QPSLimit: rate.NewLimiter(s.SendLimit, 1),
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

		if err := build.sendFn(toSend); err != nil {
			return err
		}
		lastSent = version
		return nil
	})
	if err != nil {
		panic(err) // only if dispatcher.Options were invalid
	}
	return sender
}

// Use calls `cb` with the State installed into the context, enabling the other
// methods in this package (ModifyProperties, WithStep, etc.) to update State.
//
// If `cb` returns, the final Build status will be set with GetErrorStatus.
// If `cb` panics, the final Build status will be INFRA_FAILURE.
//
// This function returns:
//   * The final sent Build.
//   * The recover()'d object in case `cb` panic'd.
//   * The error from `cb`. This will be ErrCallbackPaniced if `cb` paniced.
func (s Sink) Use(ctx context.Context, cb func(context.Context, *State) error) (final *bbpb.Build, recovered interface{}, err error) {
	initial := s.InitialBuild
	if initial == nil {
		initial = &bbpb.Build{}
	}

	build := &State{
		build:     proto.Clone(initial).(*bbpb.Build),
		initial:   proto.Clone(initial).(*bbpb.Build),
		stepNames: map[string]int{},
		sendFn:    s.SendFunc,
		ldClient:  s.LogdogClient,
	}
	build.sender = s.makeSender(ctx, build)

	if build.ldClient == nil {
		var err error
		build.ldClient, err = streamclient.New("null", "")
		if err != nil {
			panic(err)
		}
	}

	if build.build.Output == nil {
		build.build.Output = &bbpb.Build_Output{}
	}

	defer func() {
		if recovered = recover(); recovered != nil {
			logging.Errorf(ctx, "main function paniced: %s", recovered)
			if errI, ok := recovered.(error); ok {
				errors.Log(ctx, errI)
			}
			logging.Errorf(ctx, "original traceback: %s", debug.Stack())

			err = ErrCallbackPaniced
		} else if err != nil {
			logging.Errorf(ctx, "main function failed: %s", err)
			errors.Log(ctx, err)
		}

		// Do final cleanup and set state. We need to lock modMu to ensure we have
		// exclusive access to `build.build`.
		build.modMu.Lock()
		build.build.Status, build.build.StatusDetails = GetErrorStatus(err)
		build.build.EndTime = google.NewTimestamp(clock.Now(ctx))
		build.signalNewVersionLocked()

		// Close the gate; no more updates. C may already be nil if the user
		// provided a 0 rate limit or a nil send function.
		if build.sender.C != nil {
			close(build.sender.C)
			build.sender.C = nil
		}
		build.detached = true
		build.modMu.Unlock()

		build.sender.CloseAndDrain(ctx)
		final = build.build
	}()

	// Note that `recovered` and `final` are both set in the defer.
	err = cb(setState(ctx, build), build)
	return
}
