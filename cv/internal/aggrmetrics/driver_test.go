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

package aggrmetrics

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestDriver(t *testing.T) {
	t.Parallel()

	ftt.Run("Driver smoke test", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		_ = ct.SetUp(t)
		_ = New(ct.Env)
	})

	ftt.Run("Driver works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mSent := func(fields ...any) any {
			return ct.TSMonSentValue(ctx, testMetric, fields...)
		}

		d := Driver{
			aggregators: []aggregator{
				&testAggregator{name: "agg"},
			},
		}

		t.Run("No projects", func(t *ftt.Test) {
			assert.NoErr(t, d.Cron(ctx))
			assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeEmpty)
		})

		t.Run("With one active project", func(t *ftt.Test) {
			prjcfgtest.Create(ctx, "first", &cfgpb.Config{})
			assert.NoErr(t, d.Cron(ctx))

			t.Run("Reports new data", func(t *ftt.Test) {
				assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(1))
				assert.Loosely(t, mSent("first", "agg"), should.Equal(1001))
			})

			t.Run("Calling again won't report old data", func(t *ftt.Test) {
				prjcfgtest.Disable(ctx, "first")
				prjcfgtest.Create(ctx, "second", &cfgpb.Config{})
				ct.Clock.Add(1 * time.Hour)
				assert.NoErr(t, d.Cron(ctx))
				assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(1))
				assert.Loosely(t, mSent("second", "agg"), should.Equal(1001))
			})
		})

		t.Run("Multiple aggregators", func(t *ftt.Test) {
			aggregatorFoo := &testAggregator{name: "foo"}
			aggregatorBar := &testAggregator{name: "bar"}
			d.aggregators = []aggregator{
				aggregatorFoo,
				aggregatorBar,
			}

			prjcfgtest.Create(ctx, "first", &cfgpb.Config{})

			t.Run("All succeeds", func(t *ftt.Test) {
				assert.NoErr(t, d.Cron(ctx))
				assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(2))
				assert.Loosely(t, mSent("first", "foo"), should.Equal(1001))
				assert.Loosely(t, mSent("first", "bar"), should.Equal(1001))
			})

			t.Run("Fails but report partially", func(t *ftt.Test) {
				aggregatorBar.err = errors.New("something wrong")
				assert.ErrIsLike(t, d.Cron(ctx), "something wrong")
				assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.HaveLength(1))
				assert.Loosely(t, mSent("first", "foo"), should.Equal(1001))
			})

			t.Run("All failed", func(t *ftt.Test) {
				aggregatorFoo.err = transient.Tag.Apply(errors.New("foo went wrong"))
				aggregatorBar.err = errors.New("bar went wrong")
				assert.ErrIsLike(t, d.Cron(ctx), "bar went wrong") // use most serve error
				assert.Loosely(t, ct.TSMonStore.GetAll(ctx), should.BeEmpty)
			})
		})
	})
}

var testMetric = metric.NewInt("test/aggrmetrics", "test only", nil, field.String("project"), field.String("name"))

type testAggregator struct {
	name string
	err  error
	cnt  int32
}

func (t *testAggregator) metrics() []types.Metric {
	return []types.Metric{testMetric}
}

func (t *testAggregator) report(ctx context.Context, projects []string) error {
	cnt := atomic.AddInt32(&t.cnt, 1)
	if t.err != nil {
		return t.err
	}
	vals := make(map[string]int64, len(projects))
	for rank, p := range projects {
		testMetric.Set(ctx, int64(1000+1+rank), p, t.name)
	}
	logging.Debugf(ctx, "testAggregator %q [%d]: reported %d values", t.name, cnt, len(vals))
	return nil
}
