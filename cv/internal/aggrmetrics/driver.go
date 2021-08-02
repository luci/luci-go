// Copyright 2021 The LUCI Authors.
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

package aggrmetrics

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/server/tq"
)

// reportTTL limits for how long the data remains valid.
//
// If tsmon doesn't flush for this much time since the report is prepared,
// the report will be discarded.
//
// It should be longer than a typical tsmon flush interval and should account
// the fact that Driver.MinuteCron() and tsmon flush aren't synchronized.
const reportTTL = 2 * time.Minute

// New creates a new Driver for metrics aggregation.
func New(ctx context.Context, tqd *tq.Dispatcher) *Driver {
	d := &Driver{
		aggregators: []aggregator{
			&runsAggregator{},
		},
	}
	tsmon.RegisterCallbackIn(ctx, d.tsmonCallback)
	return d
}

// Driver takes care of invoking aggregators and correctly working with
// aggregated metrics, specifically resetting them after they are sent to avoid
// tsmon continuously sending of old values long after they were computed.
type Driver struct {
	aggregators []aggregator

	m              sync.Mutex
	nextReports    []reportFunc
	nextExpireTime time.Time
}

// Cron is expected to be called once per minute, e.g., by GAE cron.
//
// Although there can be more than 1 CV process, e.g., 2+ GAE instances,
// it's expected that cron will call at most 1 CV process at a time.
func (d *Driver) Cron(ctx context.Context) error {
	// Use short timeout to make overlap less likely.
	ctx, cancel := clock.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	active, err := activeProjects(ctx)
	if err != nil {
		return err
	}

	startTime := clock.Now(ctx)

	reports := make([]reportFunc, len(d.aggregators))
	errs := errors.NewLazyMultiError(len(d.aggregators))
	var wg sync.WaitGroup
	wg.Add(len(d.aggregators))
	for i, a := range d.aggregators {
		i, a := i, a
		go func() {
			defer wg.Done()
			switch f, err := a.prepare(ctx, active); {
			case err != nil:
				errs.Assign(i, err)
			default:
				reports[i] = f
			}
		}()
	}
	wg.Wait()

	// Save successfully produced reports regardless of errors.
	if reports = removeNils(reports); len(reports) > 0 {
		d.stageReports(ctx, reports, startTime.Add(reportTTL))
	}
	return common.MostSevereError(errs.Get())
}

func (d *Driver) stageReports(ctx context.Context, reports []reportFunc, expireTime time.Time) {
	d.m.Lock()
	defer d.m.Unlock()
	// Ensure we aren't overwriting newer report, just in case the prior cron
	// invocation somehow got stuck, e.g. due to a buggy aggregator.
	if !d.nextExpireTime.IsZero() {
		if d.nextExpireTime.Before(expireTime) {
			logging.Errorf(ctx, "aggrmetrics.MinuteCron was stuck since %s, newer report %s is already prepared", expireTime, d.nextExpireTime)
			return
		}
		logging.Errorf(ctx, "aggrmetrics.MinuteCron overwriting unsent report of %s with %s", d.nextExpireTime, expireTime)
	}
	d.nextReports = reports
	d.nextExpireTime = expireTime
}

// tsmonCallback resets old data from registered metrics and possibly sets new
// data.
//
// It's called by tsmon flush implementation on all CV processes,
// but the new values should normally be set on just one of them on whichever
// MinuteCron() was called last.
func (d *Driver) tsmonCallback(ctx context.Context) {
	d.m.Lock()
	defer d.m.Unlock()
	// In all cases, reset all metrics.
	d.resetMetrics(ctx)

	// Decide if a report should be made.
	switch now := clock.Now(ctx); {
	case d.nextExpireTime.IsZero():
		return
	case d.nextExpireTime.Before(now):
		logging.Warningf(ctx, "aggrmetrics dropping expired report of %s", d.nextExpireTime)
	default:
		// Do the reporting.
		for _, f := range d.nextReports {
			f(ctx)
		}
	}
	d.nextExpireTime = time.Time{}
	d.nextReports = nil
}

func (d *Driver) resetMetrics(ctx context.Context) {
	store := tsmon.GetState(ctx).Store()
	for _, a := range d.aggregators {
		for _, m := range a.metrics() {
			store.Reset(ctx, m)
		}
	}
}
