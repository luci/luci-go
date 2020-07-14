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

package ttq

import (
	"context"

	"go.chromium.org/luci/common/errors"
)

// PostProcess should be executed after the transaction completes to speed up
// the enqueueing process.
// TODO(tandrii): document.
//
// Failure to call PostProcess will not undermine correctness (see
// InstallRoutes), but will increase the latency between transaction completion
// and the task actually being executed.
//
// It is OK to call PostProcess asynchronously after an arbitrary delay
// after the transaction.
//
// The passed context.Context purpose is to respect its associated deadline (if
// any). PostProcess panics if the context has an active installed transaction
// in it.
type PostProcess func(context.Context)

// Options configures operations of shared parts of the TTQ library.
//
// All options have defaults.
// If there is a need to override or change default values, please read
// the overall rationale behind these default values below. Tuning one value
// without regard to others may lead to unreliable sweeping exactly when your
// service needs the sweeping the most.
//
// The goal is to keep up with temporary hiccups resulting in 5000 QPS creation
// rate of stale user tasks via "Sweeping" process.
//
// The Sweeping process ensures all transactionally committed tasks will have a
// corresponding Cloud Task task created. It periodically scans the database for
// which PostProcess was either not called or likely failed. For each such stale
// task, sweeping will create idempotently a Cloud Task and delete record in the
// database.
//
// Thus, default configuration should satisfy these constraints:
//  (a) each of the `Shards` primary sweeper tasks can process
//     `TasksPerSweepShare` in << `ScanInterval`;
//  (b) each primary sweeper task will trigger `S` sub-tasks, each with at most
//     `TasksPerScan` workload, so (a) applies to sub-tasks, too;
//  (c) `S` sub-tasks of a single shard can acquire/release leases within
//      `ScanInterval`. For legacy Datastore (not the Firestore-backend one)
//      there is a hard 0.5 Leases/s limit, thus
//          ScanInterval  * 0.5 > `S`
//  (d) finally, throughput must be sufficient to process through backlog in case
//      of hiccups, thus
//          Shards * (S+1) * TasksPerScan > 5000 * ScanInterval
//
// Experiments with ScanInterval=60s show:
//  (a, b) handling TasksPerScan=2048 takes ~2..6s.
//  (c) S = 9 suffices to keep contention on leases low enough to make
//      progress.
//  (d) With 16 shards * (9+1) * 2048 tasks/shard = 327K  >  5000*60 = 300K.
type Options struct {

	// Shards defines the initial number of shards for sweeping. Defaults to 16.
	//
	// If the are many stale tasks to process, each shard may be additionally
	// partitioned. See TasksPerScan.
	Shards uint

	// TasksPerScan caps maximum number of tasks that a shard task will
	// process. Defaults to 2048.
	TasksPerScan uint

	// Advanced.

	// SecondaryScanShards caps the sharding of additional scans to be performed
	// if initial scan didn't cover the whole assigned partition. In practice,
	// this matters only when database is slow or there is a huge backlog.
	// Defaults to 16.
	SecondaryScanShards uint
}

// Validate validates option values and applies defaults in place.
func (s *Options) Validate() error {
	switch {
	case s.Shards == 0:
		// 16 is chosen because initial shard partition ranges in hex are more
		// readable when debugging, but functionally value of 15 or 17 will work
		// just as well.
		s.Shards = 16
	case s.Shards > 100:
		// >100 is a sign of desperate attempt to process backlog without
		// understanding. Chances are that the real problem is somewhere else,
		// say a misconfigured helper Queue.
		return errors.New("Shards must be in [0..100] range")
	}
	if s.TasksPerScan == 0 {
		s.TasksPerScan = 2048
	}
	if s.SecondaryScanShards == 0 {
		s.SecondaryScanShards = 16
	}
	return nil
}
