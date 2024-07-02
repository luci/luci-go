// Copyright 2024 The LUCI Authors.
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

package scan

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
)

// newTSMonState builds a tsmon state with a global target.
//
// That way different processes report metrics into the same target. Processes
// need to cooperate with one another to avoid conflicts. We do it by relying on
// GAE cron overrun protection (it won't launch a cron invocation if the
// previous one is still running).
//
// Note that we purposefully do not want to retain this state between scans,
// because we rebuild it from scratch each time. This allows us to "forget"
// about tasks and bots that no longer appear in the scan.
func newTSMonState(serviceName, jobName string, mon monitor.Monitor) *tsmon.State {
	state := tsmon.NewState()
	state.SetStore(store.NewInMemory(&target.Task{
		DataCenter:  "appengine",
		ServiceName: serviceName,
		JobName:     jobName,
		HostName:    "global",
	}))
	state.InhibitGlobalCallbacksOnFlush()
	state.SetMonitor(mon)
	return state
}

// flushTSMonState flushes the metrics accumulated in the state.
//
// `ctx` here should have the default process-global tsmon state, to make sure
// stats about the flushing process itself are reported to the default tsmon
// state.
func flushTSMonState(ctx context.Context, state *tsmon.State) error {
	startTS := clock.Now(ctx)
	if err := state.ParallelFlush(ctx, nil, 32); err != nil {
		return errors.Annotate(err, "failed to flush metrics").Err()
	}
	logging.Infof(ctx, "Flushed metrics in %s", clock.Since(ctx, startTS))
	return nil
}
