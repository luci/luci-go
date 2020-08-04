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

package sweep

import (
	"context"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/tq/internal/sweep/sweeppb"
)

// Distributed implements distributed sweeping.
//
// Requires its EnqueueSweepTask callback to be configured in a way that
// enqueued tasks eventually result in ExecSweepTask call (perhaps in a
// different process).
type Distributed struct {
	// EnqueueSweepTask submits the task for execution somewhere in the fleet.
	EnqueueSweepTask func(ctx context.Context, task *sweeppb.SweepTask) error
}

// ExecSweepTask executes a previously enqueued sweep task.
func (d *Distributed) ExecSweepTask(ctx context.Context, task *sweeppb.SweepTask) error {
	// TODO(vadimsh): Implement.
	logging.Infof(ctx, "Sweeping %q", task.Partition)
	return nil
}
