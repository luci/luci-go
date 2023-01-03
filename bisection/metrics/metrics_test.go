// Copyright 2022 The LUCI Authors.
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

package metrics

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCollectGlobalMetrics(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	c, _ = tsmon.WithDummyInMemory(c)

	Convey("For running analyses", t, func() {
		createRunningAnalysis(c, 123, "chromium")
		createRunningAnalysis(c, 456, "chromeos")
		createRunningAnalysis(c, 789, "chromium")
		err := CollectGlobalMetrics(c)
		So(err, ShouldBeNil)
		So(runningAnalysesGauge.Get(c, "chromium"), ShouldEqual, 2)
		So(runningAnalysesGauge.Get(c, "chromeos"), ShouldEqual, 1)

		m, err := retrieveRunningAnalyses(c)
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string]int{
			"chromium": 2,
			"chromeos": 1,
		})
	})
}

func createRunningAnalysis(c context.Context, id int64, proj string) {
		fb := &model.LuciFailedBuild{
			Id: id,
			LuciBuild: model.LuciBuild{
				Project: proj,
			},
		}
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := testutil.CreateCompileFailure(c, fb)
		cfa := &model.CompileFailureAnalysis{
			Id: id,
			CompileFailure: datastore.KeyForObj(c, cf),
			RunStatus: pb.AnalysisRunStatus_STARTED,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
}
