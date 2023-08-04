// Copyright 2023 The LUCI Authors.
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

package bisection

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/internal/config"
	configpb "go.chromium.org/luci/bisection/proto/config"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRunBisector(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)

	Convey("No analysis", t, func() {
		err := Run(ctx, 123)
		So(err, ShouldNotBeNil)
	})

	Convey("Bisection is not enabled", t, func() {
		enableBisection(ctx, false)
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)

		err := Run(ctx, 1000)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, bisectionpb.AnalysisStatus_DISABLED)
		So(tfa.RunStatus, ShouldEqual, bisectionpb.AnalysisRunStatus_ENDED)
	})

	Convey("No primary failure", t, func() {
		enableBisection(ctx, true)
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID: 1001,
		})

		err := Run(ctx, 1001)
		So(err, ShouldNotBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, bisectionpb.AnalysisStatus_ERROR)
		So(tfa.RunStatus, ShouldEqual, bisectionpb.AnalysisRunStatus_ENDED)
	})

	Convey("Unsupported project", t, func() {
		enableBisection(ctx, true)
		tf := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			Project: "chromeos",
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:          1002,
			TestFailure: tf,
		})

		err := Run(ctx, 1002)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, bisectionpb.AnalysisStatus_UNSUPPORTED)
		So(tfa.RunStatus, ShouldEqual, bisectionpb.AnalysisRunStatus_ENDED)
	})

	Convey("Supported project", t, func() {
		enableBisection(ctx, true)
		tf := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			IsPrimary: true,
			Variant: map[string]string{
				"test_suite": "test_suite",
			},
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:          1002,
			TestFailure: tf,
		})

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		So(datastore.Put(ctx, tf), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1002)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, bisectionpb.AnalysisStatus_RUNNING)
		So(tfa.RunStatus, ShouldEqual, bisectionpb.AnalysisRunStatus_STARTED)
	})
}

func enableBisection(ctx context.Context, enabled bool) {
	testCfg := &configpb.Config{
		TestAnalysisConfig: &configpb.TestAnalysisConfig{
			BisectorEnabled: enabled,
		},
	}
	So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)
}
