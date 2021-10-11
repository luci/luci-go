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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDriver(t *testing.T) {
	t.Parallel()

	Convey("Driver smoke test", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		_ = New(ctx, ct.TQDispatcher)
	})

	Convey("Driver works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		mSent := func(fields ...interface{}) interface{} {
			return ct.TSMonSentValue(ctx, testMetric, fields...)
		}

		d := Driver{
			aggregators: []aggregator{
				&testAggregator{name: "agg"},
			},
		}
		tsmon.RegisterCallbackIn(ctx, d.tsmonCallback)
		So(tsmon.Flush(ctx), ShouldBeNil)
		So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)

		Convey("No projects", func() {
			So(d.Cron(ctx), ShouldBeNil)
			So(tsmon.Flush(ctx), ShouldBeNil)
			So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
		})

		Convey("With one project", func() {
			prjcfgtest.Create(ctx, "first", &cfgpb.Config{})
			So(d.Cron(ctx), ShouldBeNil)
			So(tsmon.Flush(ctx), ShouldBeNil)

			Convey("Reports new data", func() {
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("first", "agg"), ShouldEqual, 1001)
			})

			Convey("Next flush doesn't send the old data", func() {
				ct.Clock.Add(time.Minute)
				So(tsmon.Flush(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("Does not report old data", func() {
				ct.Clock.Add(time.Minute)
				So(d.Cron(ctx), ShouldBeNil)
				// Simulate very delayed flush.
				ct.Clock.Add(reportTTL + time.Second)
				So(tsmon.Flush(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
			})

			Convey("Reports only the newest data if a flush was delayed", func() {
				So(d.Cron(ctx), ShouldBeNil)
				// Change aggregator name to detect which data was sent.
				d.aggregators[0].(*testAggregator).name = "latest"
				ct.Clock.Add(time.Minute)
				So(d.Cron(ctx), ShouldBeNil)
				// Finally, flush.
				So(tsmon.Flush(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("first", "latest"), ShouldEqual, 1001)
				So(mSent("first", "agg"), ShouldBeNil)
			})

			Convey("With another project", func() {
				ct.Clock.Add(activeProjectsTTL)
				prjcfgtest.Create(ctx, "second", &cfgpb.Config{})
				So(d.Cron(ctx), ShouldBeNil)

				Convey("Reports data for both projects", func() {
					So(tsmon.Flush(ctx), ShouldBeNil)
					So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 2)
					So(mSent("first", "agg"), ShouldEqual, 1001)
					So(mSent("second", "agg"), ShouldEqual, 1002)

					Convey("With the second project disabled, reports just the first project", func() {
						ct.Clock.Add(activeProjectsTTL)
						prjcfgtest.Disable(ctx, "second")
						So(d.Cron(ctx), ShouldBeNil)
						So(tsmon.Flush(ctx), ShouldBeNil)
						So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
						So(mSent("first", "agg"), ShouldEqual, 1001)
					})
				})
			})

			Convey("Erroring out aggregator is ignored", func() {
				d.aggregators = append(d.aggregators, &testAggregator{name: "err", err: errors.New("oops")})
				ct.Clock.Add(time.Minute)
				So(d.Cron(ctx), ShouldErrLike, "oops")
				So(tsmon.Flush(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("first", "agg"), ShouldEqual, 1001)
				So(mSent("first", "err"), ShouldBeNil)
			})

			Convey("Another OK aggregator", func() {
				d.aggregators = append(d.aggregators, &testAggregator{name: "xxz"})
				ct.Clock.Add(time.Minute)
				So(d.Cron(ctx), ShouldBeNil)
				So(tsmon.Flush(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 2)
				So(mSent("first", "agg"), ShouldEqual, 1001)
				So(mSent("first", "xxz"), ShouldEqual, 1001)
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

func (t *testAggregator) prepare(ctx context.Context, activeProjects stringset.Set) (reportFunc, error) {
	cnt := atomic.AddInt32(&t.cnt, 1)
	if t.err != nil {
		return nil, t.err
	}

	vals := make(map[string]int64, len(activeProjects))
	for rank, p := range activeProjects.ToSortedSlice() {
		vals[p] = int64(1000 + 1 + rank)
	}
	logging.Debugf(ctx, "testAggregator %q [%d]: computed %d values", t.name, cnt, len(vals))

	return func(ctx context.Context) {
		logging.Debugf(ctx, "testAggregator %q [%d]: reporting %d values", t.name, cnt, len(vals))
		for p, v := range vals {
			testMetric.Set(ctx, v, p, t.name)
		}
	}, nil
}
