// Copyright 2018 The LUCI Authors.
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

package admin

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper"
	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

// SetupTest prepares a test environment for running mappers.
//
// Puts datastore mock into always consistent mode.
func SetupTest() (context.Context, *adminImpl, *tqtesting.Scheduler) {
	ctx, tc, _ := testutil.TestingContext()
	datastore.GetTestable(ctx).Consistent(true)

	tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		if testclock.HasTags(t, tqtesting.ClockTag) {
			tc.Add(d)
		}
	})

	dispatcher := &tq.Dispatcher{}
	mapper := &dsmapper.Controller{}
	mapper.Install(dispatcher)
	ctx, sched := tq.TestingContext(ctx, dispatcher)

	admin := &adminImpl{
		acl: func(context.Context) error { return nil },
		ctl: mapper,
	}
	admin.init()

	return ctx, admin, sched
}

// RunMapper launches a mapper and runs it till successful completion.
func RunMapper(ctx context.Context, admin *adminImpl, sched *tqtesting.Scheduler, cfg *api.JobConfig) (dsmapper.JobID, error) {
	// Launching the job creates an initial tq task.
	jobID, err := admin.LaunchJob(ctx, cfg)
	if err != nil {
		return 0, err
	}

	// Run the tq loop until there are no more pending tasks.
	sched.Run(ctx, tqtesting.StopWhenDrained())

	// Collect the result. Should be successful, otherwise RunSimulation would
	// have returned an error (it aborts on a first error from a tq task).
	state, err := admin.GetJobState(ctx, jobID)
	if err != nil {
		return 0, err
	}
	if state.Info.State != dsmapperpb.State_SUCCESS {
		return 0, errors.Fmt("expecting SUCCESS state, got %s", state.Info.State)
	}

	return dsmapper.JobID(jobID.JobId), nil
}
