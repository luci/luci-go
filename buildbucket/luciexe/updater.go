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
	"time"

	"go.chromium.org/luci/common/clock"
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
	const minSleep = time.Second         // Minimum time to sleep between calls
	const maxErrSleep = 16 * time.Second // Maximum time to sleep between retries
	const waitForNewBuild = 30 * 24 * time.Hour

	var sentBuild *pb.Build    // The last build we've successfully sent.
	var curBuild *pb.Build     // The build we are trying to send.
	var errSleep time.Duration // Exponential backoff in case of retries.
	stillRunning := true
	sleep := waitForNewBuild
	earliestNextUpdateAt := clock.Now(ctx)

	for {
		select {
		case <-ctx.Done():
			stillRunning = false
		case curBuild, stillRunning = <-builds:
		case <-clock.After(ctx, sleep): // wakes up for retry.
		}

		if !stillRunning {
			return nil // There is another final UpdateBuild call anyway.
		}
		sleep = -clock.Since(ctx, earliestNextUpdateAt)
		if sleep > 0 || curBuild == nil || curBuild == sentBuild {
			continue
		}

		switch err := doUpdate(ctx, curBuild); {
		case err == nil:
			sentBuild = curBuild
			curBuild = nil
			earliestNextUpdateAt = clock.Now(ctx).Add(minSleep)
			sleep = waitForNewBuild
			errSleep = minSleep / 2 // When we double it below it equals minSleep
		case transient.Tag.In(err):
			if errSleep = errSleep * 2; errSleep > maxErrSleep {
				errSleep = maxErrSleep
			}
			earliestNextUpdateAt = clock.Now(ctx).Add(errSleep)
			sleep = errSleep
		default:
			return err // Fatal error.
		}
	}
}
