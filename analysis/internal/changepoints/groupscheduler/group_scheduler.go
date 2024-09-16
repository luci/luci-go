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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/services/changepointgrouper"
)

// The number of weeks to schedule grouping task for in each run.
const weeksPerRun = 8

func CronHandler(ctx context.Context) error {
	currentWeek := changepoints.StartOfWeek(clock.Now(ctx))
	for i := 0; i < weeksPerRun; i++ {
		week := currentWeek.Add(-time.Duration(i) * 7 * 24 * time.Hour)
		if err := changepointgrouper.Schedule(ctx, week); err != nil {
			return errors.Annotate(err, "schedule group changepoint task week %s", week).Err()
		}
	}
	return nil
}
