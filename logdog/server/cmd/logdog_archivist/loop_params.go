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

package main

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
)

// loopParams control the outer archivist loop.
type loopParams struct {
	batchSize int64
	deadline  time.Duration
}

func (l loopParams) mkRequest(ctx context.Context) *logdog.LeaseRequest {
	return &logdog.LeaseRequest{
		MaxTasks:  l.batchSize,
		LeaseTime: ptypes.DurationProto(l.deadline),
	}
}

var (
	// loopM protects 'loop'.
	loopM = sync.Mutex{}

	// Note: these initial values are mostly bogus, they are replaced by
	// fetchLoopParams before the loop starts.
	loop = loopParams{
		// batchSize is the number of jobs to lease from taskqueue per cycle.
		//
		// TaskQueue has a limit of 10qps for leasing tasks, so the batch size must
		// be set to:
		// batchSize * 10 > (max expected stream creation QPS)
		//
		// In 2020, max stream QPS is approximately 1000 QPS
		batchSize: 500,

		// leaseTime is the amount of time to to lease the batch of tasks for.
		//
		// We need:
		// (time to process avg stream) * batchSize < leaseTime
		//
		// As of 2020, 90th percentile process time per stream is ~5s, 95th
		// percentile of the loop time is 25m.
		deadline: 40 * time.Minute,
	}
)

// grabLoopParams returns a copy of most recent value of `loop`.
func grabLoopParams() loopParams {
	loopM.Lock()
	defer loopM.Unlock()
	return loop
}

// fetchLoopParams updates `loop` based on settings in datastore (or panics).
func fetchLoopParams(ctx context.Context) {
	// Note: these are eventually controlled through /admin/portal/archivist.
	set := coordinator.GetSettings(ctx)
	loopM.Lock()
	defer loopM.Unlock()
	if set.ArchivistBatchSize != 0 {
		loop.batchSize = set.ArchivistBatchSize
	}
	if set.ArchivistLeaseTime != 0 {
		loop.deadline = set.ArchivistLeaseTime
	}
	logging.Debugf(ctx, "loop settings: batchSize:%d, leastTime:%s", loop.batchSize, loop.deadline)
}

// loopParamsUpdater updates loopParams based on setting in datastore every once
// in a while.
func loopParamsUpdater(ctx context.Context) {
	for {
		if clock.Sleep(ctx, 5*time.Minute).Err != nil {
			break //  the context is canceled, the process is exiting
		}
		fetchLoopParams(ctx)
	}
}
