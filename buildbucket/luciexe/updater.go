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

package luciexe

import (
	"context"
	"sync"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// buildUpdater sends UpdateBuildRequests to the server.
type buildUpdater struct {
	buildID    int64
	buildToken string
	builds chan *pb.Build
	client     pb.BuildsClient
}

func newBuildUpdater(buildToken string, buildID int64, client pb.BuildsClient) *buildUpdater {
	return &buildUpdater{
		client:     client,
		buildToken: buildToken,
		buildID:    buildID,
		builds:     make(chan *pb.Build),
	}
}

// BuildUpdated enqueues an UpdateBuild RPC. Assumes build will stay unchanged.
func (bu *buildUpdater) BuildUpdated(build *pb.Build) {
	bu.builds <- build
}

// UpdateBuild sends an UpdateBuildRPC.
func (bu *buildUpdater) UpdateBuild(ctx context.Context, req *pb.UpdateBuildRequest) error {
	// Insert the build token into the context.
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(buildbucket.BuildTokenHeader, bu.buildToken))
	_, err := bu.client.UpdateBuild(ctx, req)
	return err
}

// Run calls client.UpdateBuild on new b.builds. Logs transient errors and returns a fatal
// error, if any. Stops when done is closed or ctx is done.
func (bu *buildUpdater) Run(ctx context.Context, done <-chan struct{}) error {
	return bu.run(ctx, done, bu.updateBuild)
}

func (bu *buildUpdater) run(
	ctx context.Context,
	done <-chan struct{},
	update func(context.Context, *pb.Build) error,
) error {
	cond := sync.NewCond(&sync.Mutex{})
	// protected by cond.L
	var state struct {
		latest    *pb.Build
		latestVer int
		done      bool
	}

	// Listen to new requests.
	go func() {
		locked := func(f func()) {
			cond.L.Lock()
			f()
			cond.L.Unlock()
			cond.Signal()
		}

		for {
			select {
			case build := <-bu.builds:
				locked(func() {
					state.latest = build
					state.latestVer++
				})

			case <-ctx.Done():
				locked(func() { state.done = true })
			case <-done:
				locked(func() { state.done = true })
			}
		}
	}()

	// Send requests

	var sentVer int
	// how long did we wait after most recent update call
	var errSleep time.Duration
	var lastRequestTime time.Time
	for {
		// Ensure at least 1s between calls.
		if !lastRequestTime.IsZero() {
			ellapsed := clock.Since(ctx, lastRequestTime)
			if d := time.Second - ellapsed; d > 0 {
				clock.Sleep(clock.Tag(ctx, "update-build-distance"), d)
			}
		}

		// Wait for news.
		cond.L.Lock()
		if sentVer == state.latestVer && !state.done {
			cond.Wait()
		}
		local := state
		cond.L.Unlock()

		var err error
		if sentVer != local.latestVer {
			lastRequestTime = clock.Now(ctx)

			err = update(ctx, local.latest)
			switch status.Code(errors.Unwrap(err)) {
			case codes.OK:
				errSleep = 0
				sentVer = local.latestVer

			case codes.InvalidArgument:
				// This is fatal.
				return err

			default:
				// Hope another future request will succeed.
				// There is another final UpdateBuild call anyway.
				logging.Errorf(ctx, "failed to update build: %s", err)

				// Sleep.
				if errSleep == 0 {
					errSleep = time.Second
				} else if errSleep < 16*time.Second {
					errSleep *= 2
				}
				logging.Debugf(ctx, "will sleep for %s", errSleep)

				clock.Sleep(clock.Tag(ctx, "update-build-error"), errSleep)
			}
		}

		if local.done {
			return err
		}
	}

}

// UpdateBuild updates a build on the buildbucket server. Includes a build token in the
// request.
func (bu *buildUpdater) updateBuild(ctx context.Context, build *pb.Build) error {
	buildClone := proto.Clone(build).(*pb.Build)
	buildClone.Status = pb.Status_STARTED
	return bu.UpdateBuild(ctx, &pb.UpdateBuildRequest{
		Build: buildClone,
		UpdateMask: &field_mask.FieldMask{Paths: []string{
			"build.output.properties",
			"build.steps",
			"build.status",
		}},
	})
}
