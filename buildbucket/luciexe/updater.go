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
//
// Every Build object pointer must be unique in the `builds` channel. Duplicate
// or re-used pointers may be ignored. Build objects must also be immutable once
// placed in this channel.
func (doUpdate buildUpdater) Run(ctx context.Context, builds <-chan *pb.Build) error {
	cond := sync.NewCond(&sync.Mutex{})
	locked := func(f func()) {
		cond.L.Lock()
		defer func() {
			cond.L.Unlock()
			cond.Signal()
		}()
		f()
	}

	// protected by cond.L
	var state struct {
		latest *pb.Build
		done   bool
	}

	// Listen to new requests.
	go func() {
		for {
			select {
			case build, ok := <-builds:
				if !ok {
					locked(func() { state.done = true })
					return
				}
				locked(func() { state.latest = build })

			case <-ctx.Done():
				locked(func() { state.done = true })
				return
			}
		}
	}()

	// The last we successfully sent.
	var lastSent *pb.Build

	// how long did we wait after most recent update call
	var errSleep time.Duration
	const minSleep = time.Second      // Minimum time to sleep between calls
	const maxSleep = 16 * time.Second // Maximum time to sleep between calls

	var lastRequestTime time.Time
	for {
		// Ensure minSleep between calls.
		if !lastRequestTime.IsZero() {
			elapsed := clock.Since(ctx, lastRequestTime)
			if d := minSleep - elapsed; d > 0 {
				clock.Sleep(clock.Tag(ctx, "update-build-distance"), d)
			}
		}

		// Wait for news.
		cond.L.Lock()
		if lastSent == state.latest && !state.done {
			cond.Wait()
		}
		local := state
		cond.L.Unlock()

		var err error
		if lastSent != local.latest {
			lastRequestTime = clock.Now(ctx)

			err = doUpdate(ctx, local.latest)
			if err == nil {
				errSleep = minSleep / 2 // When we double it below it equals minSleep
				lastSent = local.latest
			} else if transient.Tag.In(err) {
				// Hope another future request will succeed.
				// There is another final UpdateBuild call anyway.
				logging.Errorf(ctx, "failed to update build: %s", err)

				if errSleep != maxSleep {
					errSleep *= 2
				}
				if errSleep > maxSleep {
					errSleep = maxSleep
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
