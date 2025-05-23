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

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
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
		metrics.Public.PendingRunCount,
		metrics.Public.PendingRunDuration,
		metrics.Public.MaxPendingRunAge,
		metrics.Public.ActiveRunCount,
		metrics.Public.ActiveRunDuration,
	}
}

// report implements aggregator interface.
func (r *runsAggregator) report(ctx context.Context, projects []string) error {
	eg, ectx := errgroup.WithContext(ctx)
	var pendingRunKeys []*datastore.Key
	var activeRunKeys []*datastore.Key
	var runStats *runStats
	eg.Go(func() (err error) {
		q := datastore.NewQuery(common.RunKind).Eq("Status", run.Status_PENDING)
		switch pendingRunKeys, err = loadRunKeys(ectx, q, maxRuns+1); {
		case err != nil:
			return err
		case len(pendingRunKeys) == maxRuns+1:
			// Outright refuse sending incomplete data.
			logging.Errorf(ctx, "FIXME: too many pending runs (>%d) to report aggregated metrics for", maxRuns)
			return errors.New("too many pending Runs")
		default:
			return nil
		}
	})
	eg.Go(func() (err error) {
		q := datastore.NewQuery(common.RunKind).
			Lt("Status", run.Status_ENDED_MASK).
			Gt("Status", run.Status_PENDING)
		switch activeRunKeys, err = loadRunKeys(ectx, q, maxRuns+1); {
		case err != nil:
			return err
		case len(activeRunKeys) == maxRuns+1:
			// Outright refuse sending incomplete data.
			logging.Errorf(ctx, "FIXME: too many active runs (>%d) to report aggregated metrics for", maxRuns)
			return errors.New("too many active Runs")
		default:
			return nil
		}
	})
	eg.Go(func() (err error) {
		runStats, err = initRunStats(ectx, projects)
		return err
	})
	if err := eg.Wait(); err != nil {
		return err
	}

	eg, ectx = errgroup.WithContext(ctx)
	now := clock.Now(ctx)
	eg.Go(func() error {
		return iterRuns(ectx, pendingRunKeys, maxRunsWorkingSet, func(r *run.Run) {
			runStats.addPending(r, now)
		})
	})
	eg.Go(func() error {
		return iterRuns(ectx, activeRunKeys, maxRunsWorkingSet, func(r *run.Run) {
			runStats.addActive(r, now)
		})
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	runStats.report(ctx)
	return nil
}

type runStats struct {
	pendingRunCounts    map[runFields]int64
	pendingRunDurations map[runFields]*distribution.Distribution
	maxPendingRunAge    map[runFields]time.Duration
	activeRunCounts     map[runFields]int64
	activeRunDurations  map[runFields]*distribution.Distribution
}

type runFields struct {
	project, configGroup, mode string
}

func makeRunFields(r *run.Run) runFields {
	return runFields{
		project:     r.ID.LUCIProject(),
		configGroup: r.ConfigGroupID.Name(),
		mode:        string(r.Mode),
	}
}

func (f runFields) toMetricFields() []any {
	return []any{f.project, f.configGroup, f.mode}
}

func initRunStats(ctx context.Context, projects []string) (*runStats, error) {
	rs := &runStats{
		pendingRunCounts:    make(map[runFields]int64),
		pendingRunDurations: make(map[runFields]*distribution.Distribution),
		maxPendingRunAge:    make(map[runFields]time.Duration),
		activeRunCounts:     make(map[runFields]int64),
		activeRunDurations:  make(map[runFields]*distribution.Distribution),
	}
	var rsMu sync.Mutex
	// TODO: Support `GetLatestMetas` and `GetConfigGroups` in prjcfg package
	// and use those APIs instead of parallel.WorkPool to reduce the call to
	// datastore.
	err := parallel.WorkPool(min(8, len(projects)), func(work chan<- func() error) {
		for _, project := range projects {
			work <- func() error {
				switch meta, err := prjcfg.GetLatestMeta(ctx, project); {
				case err != nil:
					return err
				case meta.Status != prjcfg.StatusEnabled:
					// race condition: by the time Project config is loaded, the Project
					// is disabled, skipping this Project as we won't expect any
					// non-ended runs from this Project. Even if there's any, they will
					// be ended very soon.
					return nil
				default:
					rsMu.Lock()
					defer rsMu.Unlock()
					// pre-populate all combinations of
					// project+config_group+(DRY_RUN|FULL_RUN) so that zero value is
					// reported.
					for _, cgName := range meta.ConfigGroupNames {
						for _, standardMode := range []run.Mode{run.DryRun, run.FullRun} {
							f := runFields{
								project:     project,
								configGroup: cgName,
								mode:        string(standardMode),
							}
							rs.activeRunCounts[f] = 0
							rs.activeRunDurations[f] = distribution.New(metrics.Public.ActiveRunDuration.Bucketer())
							rs.pendingRunCounts[f] = 0
							rs.pendingRunDurations[f] = distribution.New(metrics.Public.PendingRunDuration.Bucketer())
							rs.maxPendingRunAge[f] = time.Duration(0)
						}
					}
					return nil
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (rs *runStats) addPending(r *run.Run, now time.Time) {
	switch {
	case r.Status != run.Status_PENDING:
		// Since the Run is loaded after the query, the Run might change the status.
		// Ignore this type of runs.
		return
	case r.CreateTime.After(now):
		// This should be rare, yet may happen if this process' time is
		// somewhat behind time of a concurrent process which has just created a new
		// Run. It's better to skip this newly created Run for later than report a
		// negative duration for it.
		return
	}
	runFields := makeRunFields(r)
	rs.pendingRunCounts[runFields] = rs.pendingRunCounts[runFields] + 1
	if _, ok := rs.pendingRunDurations[runFields]; !ok {
		rs.pendingRunDurations[runFields] = distribution.New(metrics.Public.PendingRunDuration.Bucketer())
	}
	dur := now.Sub(r.CreateTime)
	rs.pendingRunDurations[runFields].Add(float64(dur.Milliseconds()))
	if rs.maxPendingRunAge[runFields] == 0 || dur > rs.maxPendingRunAge[runFields] {
		rs.maxPendingRunAge[runFields] = dur
	}
}

func (rs *runStats) addActive(r *run.Run, now time.Time) {
	switch {
	case run.IsEnded(r.Status):
		// Since the Run is loaded after the query, the Run may be already finished.
		// Such Runs are no longer considered active and shouldn't be reported.
		return
	case r.StartTime.After(now):
		// This should be rare, yet may happen if this process' time is
		// somewhat behind time of a concurrent process which has just started a new
		// Run. It's better to skip this newly started Run for later than report a
		// negative duration for it.
		return
	}
	runFields := makeRunFields(r)
	rs.activeRunCounts[runFields] = rs.activeRunCounts[runFields] + 1
	if _, ok := rs.activeRunDurations[runFields]; !ok {
		rs.activeRunDurations[runFields] = distribution.New(metrics.Public.ActiveRunDuration.Bucketer())
	}
	rs.activeRunDurations[runFields].Add(now.Sub(r.StartTime).Seconds())
}

func (rs *runStats) report(ctx context.Context) {
	for fields, cnt := range rs.pendingRunCounts {
		metrics.Public.PendingRunCount.Set(ctx, cnt, fields.toMetricFields()...)
	}
	for fields, dist := range rs.pendingRunDurations {
		metrics.Public.PendingRunDuration.Set(ctx, dist, fields.toMetricFields()...)
	}
	for fields, dur := range rs.maxPendingRunAge {
		metrics.Public.MaxPendingRunAge.Set(ctx, dur.Milliseconds(), fields.toMetricFields()...)
	}
	for fields, cnt := range rs.activeRunCounts {
		metrics.Public.ActiveRunCount.Set(ctx, cnt, fields.toMetricFields()...)
	}
	for fields, dist := range rs.activeRunDurations {
		metrics.Public.ActiveRunDuration.Set(ctx, dist, fields.toMetricFields()...)
	}
}

// loadRunKeys returns only the keys of the Runs matching the given query.
func loadRunKeys(ctx context.Context, q *datastore.Query, limit int32) ([]*datastore.Key, error) {
	q = q.Limit(limit).KeysOnly(true)
	var out []*datastore.Key
	switch err := datastore.GetAll(ctx, q, &out); {
	case ctx.Err() != nil:
		logging.Warningf(ctx, "%s while fetching %s", ctx.Err(), q)
		return nil, ctx.Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch Runs").Tag(transient.Tag).Err()
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
