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
	"math"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

var (
	// TODO(tandrii): remove "internal" from metrics below and make it available
	// to users once we are comfortable with its target & fields.
	metricActiveRunsCount = metric.NewInt(
		"cv/internal/runs/active/count",
		"Count of active Runs.",
		nil,
		field.String("project"),
		field.String("status"),
	)
	metricActiveRunsDurationsS = metric.NewNonCumulativeDistribution(
		"cv/internal/runs/active/durations",
		"Ages of active Runs",
		&types.MetricMetadata{Units: types.Seconds},
		// Bucketer for 1s .. ~7d range.
		//
		// Not accurate above 1h.
		// TODO(tandrii): add another metric with a fixed width bucketer spanning
		// range up to 2 hours, which is what most projects care about.
		distribution.GeometricBucketer(math.Pow(10, 0.06), 100),
		field.String("project"),
	)
)

const (
	// maxRuns limits how many Runs can probably be processed in-process
	// reasonably well given 10s minute deadline.
	//
	// If there are actually equal or more than this many Runs, then
	// runsAggregator will refuse to work to avoid giving incorrect / partial
	// data.
	maxRuns = 2000
	// maxRunsWorkingSet limits how many Runs are loaded into RAM at the same
	// time.
	maxRunsWorkingSet = 500
)

type runsAggregator struct {
}

// metrics implements aggregator interface.
func (r *runsAggregator) metrics() []types.Metric {
	return []types.Metric{
		metricActiveRunsCount,
		metricActiveRunsDurationsS,
	}
}

// metrics implements aggregator interface.
func (r *runsAggregator) prepare(ctx context.Context, activeProjects stringset.Set) (reportFunc, error) {
	now := clock.Now(ctx)
	keys, err := loadActiveRuns(ctx, maxRuns+1)
	switch {
	case err != nil:
		return nil, err
	case len(keys) == maxRuns+1:
		// Outright refuse sending incomplete data.
		logging.Errorf(ctx, "FIXME: too many active runs (>%d) to report aggregated metrics for", maxRuns)
		return nil, errors.New("too many active Runs")
	}
	stats := make(map[string]*projectStat, len(activeProjects))
	for p := range activeProjects {
		stats[p] = newProjectStat()
	}
	err = iterRuns(ctx, keys, maxRunsWorkingSet, func(r *run.Run) {
		name := r.ID.LUCIProject()
		p, exists := stats[name]
		if !exists {
			// Although rare, this can happen if a new project has just been added.
			p = newProjectStat()
			stats[name] = p
		}
		p.add(r, now)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		for project, ps := range stats {
			ps.report(ctx, project)
		}
	}, nil
}

type projectStat struct {
	byStatus      map[run.Status]int64
	sinceCreation *distribution.Distribution
}

func newProjectStat() *projectStat {
	return &projectStat{
		// Always set Status_RUNNING to 0, as this way all projects will always be
		// reported.
		byStatus:      map[run.Status]int64{run.Status_RUNNING: 0},
		sinceCreation: distribution.New(metricActiveRunsDurationsS.Bucketer()),
	}
}

func (p *projectStat) add(r *run.Run, now time.Time) {
	switch {
	case run.IsEnded(r.Status):
		// Since the Run is loaded after the query, the Run may be already finished.
		// Such Runs are no longer active and shouldn't be reported.
		return
	case r.CreateTime.After(now):
		// This should be rare, yet may happen if this process' time is
		// somewhat behind time of a concurrent process which has just created a new
		// Run. It's better to skip this newly created Run for later than report a
		// negative duration for it.
		return
	}
	p.byStatus[r.Status]++
	p.sinceCreation.Add(now.Sub(r.CreateTime).Seconds())
}

func (p *projectStat) report(ctx context.Context, project string) {
	for code, cnt := range p.byStatus {
		status := run.Status_name[int32(code)]
		metricActiveRunsCount.Set(ctx, cnt, project, status)
	}
	metricActiveRunsDurationsS.Set(ctx, p.sinceCreation, project)
}

// loadActiveRuns returns only the keys of the active Runs.
//
// This is a cheap query in Datastore, both in terms of time and $ cost.
func loadActiveRuns(ctx context.Context, limit int32) ([]*datastore.Key, error) {
	q := datastore.NewQuery(run.RunKind).Limit(limit).KeysOnly(true).Lt("Status", run.Status_ENDED_MASK)
	var out []*datastore.Key
	switch err := datastore.GetAll(ctx, q, &out); {
	case ctx.Err() != nil:
		logging.Warningf(ctx, "%s while fetching %s", ctx.Err(), q)
		return nil, ctx.Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch active Runs").Tag(transient.Tag).Err()
	case len(out) == int(limit):
		logging.Errorf(ctx, "FIXME: %s fetched exactly the limit of Runs; reported data is incomplete", q)
	}
	return out, nil
}

// iterRuns calls clbk function on each Run represented by its key.
//
// Loads Runs in batches to avoid excessive RAM consumption.
func iterRuns(ctx context.Context, keys []*datastore.Key, bufSize int, clbk func(r *run.Run)) error {
	batch := make([]*run.Run, 0, min(bufSize, len(keys)))
	for len(keys) > 0 {
		batch = batch[:0]
		for i := 0; i < min(len(keys), cap(batch)); i++ {
			batch = append(batch, &run.Run{ID: common.RunID(keys[i].StringID())})
		}
		keys = keys[len(batch):]
		if err := datastore.Get(ctx, batch); err != nil {
			return errors.Annotate(err, "failed to load Runs").Tag(transient.Tag).Err()
		}
		for _, r := range batch {
			clbk(r)
		}
	}
	return nil
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
