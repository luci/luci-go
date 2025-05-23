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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/cv/internal/common"
)

// New creates a new Driver for metrics aggregation.
func New(env *common.Env) *Driver {
	d := &Driver{
		aggregators: []aggregator{
			&runsAggregator{},
			&pmReporter{},
			&builderPresenceAggregator{env: env},
		},
	}
	return d
}

// Driver takes care of invoking aggregators and correctly working with
// aggregated metrics.
type Driver struct {
	aggregators []aggregator
}

type aggregator interface {
	// metrics must return of metrics set by the aggregator, which should be reset
	// after the tsmon flush.
	metrics() []types.Metric

	// report should aggregate CV's state and report the tsmon metrics for the
	// provided projects.
	report(ctx context.Context, projects []string) error
}

// Cron reports all the aggregated metrics for all active projects.
//
// Resets all known reporting metrics at the beginning. It is expected to be
// called once per minute, e.g., by GAE cron.
func (d *Driver) Cron(ctx context.Context) error {
	d.resetMetrics(ctx)
	active, err := activeProjects(ctx)
	if err != nil {
		return err
	}

	errs := errors.NewLazyMultiError(len(d.aggregators))
	var wg sync.WaitGroup
	wg.Add(len(d.aggregators))
	for i, a := range d.aggregators {
		go func() {
			defer wg.Done()
			if err := a.report(ctx, active); err != nil {
				errs.Assign(i, err)
			}
		}()
	}
	wg.Wait()

	return common.MostSevereError(errs.Get())
}

func (d *Driver) resetMetrics(ctx context.Context) {
	store := tsmon.GetState(ctx).Store()
	for _, a := range d.aggregators {
		for _, m := range a.metrics() {
			store.Reset(ctx, m)
		}
	}
}
