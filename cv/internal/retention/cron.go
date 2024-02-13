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

package retention

import (
	"context"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
)

// RegisterCrons registers cron jobs for data retention.
func RegisterCrons(tqd *tq.Dispatcher, rm rm) {
	registerWipeoutRunsTask(tqd, rm)
	registerWipeoutCLsTask(tqd)
	registerWipeoutTryjobsTask(tqd)
	cron.RegisterHandler("data-retention-runs", func(ctx context.Context) error {
		return scheduleWipeoutRuns(ctx, tqd)
	})
	cron.RegisterHandler("data-retention-cls", func(ctx context.Context) error {
		return scheduleWipeoutCLTasks(ctx, tqd)
	})
	cron.RegisterHandler("data-retention-tryjobs", func(ctx context.Context) error {
		return scheduleWipeoutTryjobsTasks(ctx, tqd)
	})
}

// rm encapsulates communication between data retention jobs and run manager.
type rm interface {
	// PokeNow tells RunManager to check its own state immediately.
	PokeNow(ctx context.Context, runID common.RunID) error
}
