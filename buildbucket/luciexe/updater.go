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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// buildUpdater sends UpdateBuildRequests to the server.
type buildUpdater func(ctx context.Context, build *pb.Build) error

// Run calls the updater on builds from the input channel. Logs transient errors and
// returns a fatal error, if any. Stops when ctx is done.
func (bu buildUpdater) Run(ctx context.Context, builds <-chan *pb.Build) error {
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
			case build := <-builds:
				locked(func() {
					state.latest = build
					state.latestVer++
				})

			case <-ctx.Done():
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

			doUpdate := bu
			err = doUpdate(ctx, local.latest)
			if err == nil {
				errSleep = 0
				sentVer = local.latestVer
			} else if transient.Tag.In(err) {
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
			} else {
				// fatal
				return err
			}
		}

		if local.done {
			return err
		}
	}
}
