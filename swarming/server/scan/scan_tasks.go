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
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

// TaskVisitor examines tasks, **sequentially**.
//
// Should keep track of errors internally during the scan, reporting them only
// in the end in Finalize.
type TaskVisitor interface {
	// Prepare prepares the visitor state.
	//
	// It should initialize the state used by Visit.
	Prepare(ctx context.Context)

	// Visit is called for every visited task.
	//
	// Called sequentially from a single goroutine.
	Visit(ctx context.Context, task *model.TaskResultSummary)

	// Finalize is called once the scan is done.
	//
	// It is passed an error if the scan was incomplete. If the scan was complete,
	// (i.e. Visit visited all tasks), it receives nil.
	//
	// The returned error will be reported as an overall scan error.
	Finalize(ctx context.Context, scanErr error) error
}

// ActiveTasks visits all pending and running tasks.
//
// Returns a multi-error with the scan error (if any) and errors from all
// visitors' finalizers (if any) in arbitrary order.
func ActiveTasks(ctx context.Context, visitors []TaskVisitor) error {
	q := model.TaskResultSummaryQuery().
		Lte("state", apipb.TaskState_PENDING).
		Gte("state", apipb.TaskState_RUNNING)

	startTS := clock.Now(ctx)
	total := 0

	for _, v := range visitors {
		v.Prepare(ctx)
	}

	scanErr := datastore.RunBatch(ctx, 1200, q, func(trs *model.TaskResultSummary) error {
		for _, v := range visitors {
			v.Visit(ctx, trs)
		}
		total++
		return nil
	})
	if scanErr != nil {
		logging.Errorf(ctx, "Scan failed after %s. Visited tasks: %d", clock.Since(ctx, startTS), total)
		scanErr = errors.Annotate(scanErr, "scanning TaskResultSummary").Err()
	} else {
		logging.Infof(ctx, "Scan done in %s. Total visited tasks: %d", clock.Since(ctx, startTS), total)
	}

	// Run finalizers in parallel, they may do slow IO. Collect all errors.
	errs := make(chan error, len(visitors))
	for _, v := range visitors {
		v := v
		go func() { errs <- v.Finalize(ctx, scanErr) }()
	}
	var merr errors.MultiError
	merr.MaybeAdd(scanErr)
	for range visitors {
		merr.MaybeAdd(<-errs)
	}
	return merr.AsError()
}
