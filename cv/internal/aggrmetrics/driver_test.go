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
		_ = ct.SetUp(t)
		_ = New(ct.Env)
	})

	Convey("Driver works", t, func() {
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

		Convey("No projects", func() {
			So(d.Cron(ctx), ShouldBeNil)
			So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
		})

		Convey("With one active project", func() {
			prjcfgtest.Create(ctx, "first", &cfgpb.Config{})
			So(d.Cron(ctx), ShouldBeNil)

			Convey("Reports new data", func() {
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("first", "agg"), ShouldEqual, 1001)
			})

			Convey("Calling again won't report old data", func() {
				prjcfgtest.Disable(ctx, "first")
				prjcfgtest.Create(ctx, "second", &cfgpb.Config{})
				ct.Clock.Add(1 * time.Hour)
				So(d.Cron(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("second", "agg"), ShouldEqual, 1001)
			})
		})

		Convey("Multiple aggregators", func() {
			aggregatorFoo := &testAggregator{name: "foo"}
			aggregatorBar := &testAggregator{name: "bar"}
			d.aggregators = []aggregator{
				aggregatorFoo,
				aggregatorBar,
			}

			prjcfgtest.Create(ctx, "first", &cfgpb.Config{})

			Convey("All succeeds", func() {
				So(d.Cron(ctx), ShouldBeNil)
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 2)
				So(mSent("first", "foo"), ShouldEqual, 1001)
				So(mSent("first", "bar"), ShouldEqual, 1001)
			})

			Convey("Fails but report partially", func() {
				aggregatorBar.err = errors.New("something wrong")
				So(d.Cron(ctx), ShouldErrLike, "something wrong")
				So(ct.TSMonStore.GetAll(ctx), ShouldHaveLength, 1)
				So(mSent("first", "foo"), ShouldEqual, 1001)
			})

			Convey("All failed", func() {
				aggregatorFoo.err = transient.Tag.Apply(errors.New("foo went wrong"))
				aggregatorBar.err = errors.New("bar went wrong")
				So(d.Cron(ctx), ShouldErrLike, "bar went wrong") // use most serve error
				So(ct.TSMonStore.GetAll(ctx), ShouldBeEmpty)
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
