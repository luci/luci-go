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
	"net/url"
	"strings"
	"time"

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
// Only the Queue name and BaseURL must be specified.
// All other options have defaults.
//
// If there is a need to override or change default values, please read
// the overall rationale behind these default values below. Tuning one value
// without regard to others may lead to unreliable sweeping exactly when your
// services needs the sweeping the most.
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
//     `TasksPerSweepShard` workload, so (a) applies to sub-tasks, too;
//  (c) `S` sub-tasks of a single shard can acquire/release leases within
//      `ScanInterval`. For legacy Datastore (not the Firestore-backend one)
//      there is a hard 0.5 Leases/s limit, thus
//          ScanInterval  * 0.5 > `S`
//  (d) finally, throughput must be sufficient to process through backlog in case
//      of hiccups, thus
//          Shards * (S+1) * TasksPerSweepShard > 5000 * ScanInterval
//
// Experiments show:
//  (a, b) handling TasksPerSweepShard=2048 takes ~2..6s.
//  (c) S = 9 suffices to keep contention on leases low enough to make
//      progress.
//  (d) With 16 shards * (9+1) * 2048 tasks/shard = 327K  >  5000*60 = 300K.
type Options struct {

	// BaseURL is the URL under which the TTQ library handlers will be installed.
	// Required.
	//
	// The URL must be externally callable by Cloud Tasks.
	// The TTQ library reserves full use of this URL prefix.
	// The URL must have a path component. Fragment and query components as well
	// as trailing slash are not allowed in the URL.
	//
	// When calling InstallRoutes of the specific implementation,
	// TTQ library will install handlers on the path component of the URL.
	// TODO(tandrii): add a PathPrefix option to avoid this limitation.
	//
	// For example, "https://my.example.com/internal/ttq".
	//
	// You must configure a cron job to issue HTTP GET on `BaseURL + "/cron"`
	// at least every `ScanInterval` (see doc below). No authentication is
	// required.
	// Continuing the example above, cron should issue
	// "GET https://my.example.com/internal/ttq/cron".
	BaseURL string

	// Queue is the full name of the Cloud Tasks queue to be used for sweeping.
	// Required.
	//
	// Format: `projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID`.
	// The queue must already exist with a throughput of at least 10 QPS.
	//
	// If the same queue is used for other purposes, beware that this may increase
	// the latency of the sweeping process. Thus, a dedicated queue for sweeping
	// is recommended.
	Queue string

	// ScanInterval defines how frequently the database will be scanned for stale
	// tasks.
	//
	// Default is 1 minute. Minimum is 1 minute.
	// The value will be truncated to the minute precision.
	ScanInterval time.Duration

	// Shards defines the initial number of shards for sweeping. Defaults to 16.
	//
	// If the are many stale tasks to process, each shard may be additionally
	// partitioned. See TasksPerSweepShard.
	Shards uint

	// TasksPerSweepShard caps maximum number of tasks that a shard task will
	// process. Defaults to 2048.
	TasksPerSweepShard uint
}

// Validate validates option values and applies defaults in place.
func (s *Options) Validate() error {
	switch u, err := url.Parse(s.BaseURL); {
	case s.BaseURL == "":
		return errors.New("BaseURL is required")
	case err != nil:
		return errors.Annotate(err, "invalid BaseURL %q", s.BaseURL).Err()
	case u.Fragment != "" || u.RawQuery != "":
		return errors.Reason("BaseURL %q must not have fragment and query components", s.BaseURL).Err()
	case u.Path == "":
		return errors.Reason("BaseURL %q must have path component", s.BaseURL).Err()
	case strings.HasSuffix(u.Path, "/"):
		return errors.Reason("BaseURL %q must not end with /", s.BaseURL).Err()
	}

	// Quick check for typical mistakes in the Queue now.
	// Actual validity will only be known once cronjob creates the first sweeping
	// tasks.
	switch qs := strings.Split(s.Queue, "/"); {
	case s.Queue == "":
		return errors.New("Queue is required")
	case len(qs) != 6 || qs[0] != "projects" || qs[2] != "locations" || qs[4] != "queues":
		return errors.Reason("Queue %q must be in format 'projects/PROJECT_ID/locations/LOCATION_ID/queues/QUEUE_ID'", s.Queue).Err()
	}

	switch {
	case s.ScanInterval == 0:
		s.ScanInterval = time.Minute
	case s.ScanInterval < time.Minute:
		return errors.New("ScanInterval must be at least 1 Minute")
	default:
		s.ScanInterval = s.ScanInterval.Truncate(sweepScanIntervalGranularity)
	}
	switch {
	case s.Shards == 0:
		// 16 is chosen because initial shard partition ranges in hex are more
		// readable when debugging, but functionally value of 15 or 17 will work
		// just as well.
		s.Shards = 16
	case s.Shards > 100:
		// >128 is a sign of desperate attempt to process backlog without
		// understanding. Chances are that the real problem is somewhere else,
		// say a misconfigured helper Queue.
		return errors.New("Shards must be in [0..100] range")
	}
	if s.TasksPerSweepShard == 0 {
		s.TasksPerSweepShard = 2048
	}
	return nil
}

const (
	// sweepScanIntervalGranularity is used for deduplication of sweeping shards.
	// See also SweepOptions.ScanInterval.
	sweepScanIntervalGranularity = time.Minute
)
