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

// Package groupscheduler schedules group changepoints tasks.
package groupscheduler

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

func TestScheduleGroupChangepoints(t *testing.T) {
	ctx := context.Background()
	ctx, taskScheduler := tq.TestingContext(ctx, nil)
	now := time.Date(2024, time.September, 16, 4, 4, 4, 4, time.UTC)
	ctx, _ = testclock.UseTime(ctx, now)

	ftt.Run("e2e", t, func(t *ftt.Test) {
		err := scheduleGroupingTasks(ctx)
		assert.NoErr(t, err)
		tasks := taskScheduler.Tasks().Payloads()
		assert.That(t, len(tasks), should.Equal(8))
		assert.That(t, tasks[0].(*taskspb.GroupChangepoints), should.Match(
			&taskspb.GroupChangepoints{
				Week:         timestamppb.New(time.Date(2024, time.September, 15, 0, 0, 0, 0, time.UTC)),
				ScheduleTime: timestamppb.New(now),
			},
		))
		assert.That(t, tasks[1].(*taskspb.GroupChangepoints), should.Match(
			&taskspb.GroupChangepoints{
				Week:         timestamppb.New(time.Date(2024, time.September, 8, 0, 0, 0, 0, time.UTC)),
				ScheduleTime: timestamppb.New(now),
			},
		))
		assert.That(t, tasks[2].(*taskspb.GroupChangepoints), should.Match(
			&taskspb.GroupChangepoints{
				Week:         timestamppb.New(time.Date(2024, time.September, 1, 0, 0, 0, 0, time.UTC)),
				ScheduleTime: timestamppb.New(now),
			},
		))
		assert.That(t, tasks[3].(*taskspb.GroupChangepoints), should.Match(
			&taskspb.GroupChangepoints{
				Week:         timestamppb.New(time.Date(2024, time.August, 25, 0, 0, 0, 0, time.UTC)),
				ScheduleTime: timestamppb.New(now),
			},
		))
	})
}
