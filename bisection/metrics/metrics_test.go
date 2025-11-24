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
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	tspb "go.chromium.org/luci/tree_status/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCollectGlobalMetrics(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	c, _ = tsmon.WithDummyInMemory(c)

	ftt.Run("running compile analyses", t, func(t *ftt.Test) {
		createRunningAnalysis(c, t, 123, "chromium", model.PlatformLinux)
		createRunningAnalysis(c, t, 456, "chromeos", model.PlatformLinux)
		createRunningAnalysis(c, t, 789, "chromium", model.PlatformLinux)
		err := collectMetricsForRunningAnalyses(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runningAnalysesGauge.Get(c, "chromium", "compile"), should.Equal(2))
		assert.Loosely(t, runningAnalysesGauge.Get(c, "chromeos", "compile"), should.Equal(1))

		m, err := retrieveRunningAnalyses(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, m, should.Match(map[string]int{
			"chromium": 2,
			"chromeos": 1,
		}))
	})

	ftt.Run("running test analyses", t, func(t *ftt.Test) {
		testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
			ID:        1000,
			Project:   "chromium",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
			ID:        1001,
			Project:   "chromium",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
			ID:        1002,
			Project:   "chromeos",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
			ID:        1003,
			Project:   "chromeos",
			RunStatus: pb.AnalysisRunStatus_ENDED,
		})
		err := collectMetricsForRunningAnalyses(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runningAnalysesGauge.Get(c, "chromium", "test"), should.Equal(2))
		assert.Loosely(t, runningAnalysesGauge.Get(c, "chromeos", "test"), should.Equal(1))

		m, err := retrieveRunningAnalyses(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, m, should.Match(map[string]int{
			"chromium": 2,
			"chromeos": 1,
		}))
	})

	ftt.Run("For running reruns", t, func(t *ftt.Test) {
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)
		testutil.UpdateIndices(c)

		// Create a rerun for chromium
		cfa1 := createRunningAnalysis(c, t, 123, "chromium", model.PlatformLinux)

		rrBuild1 := &model.CompileRerunBuild{
			LuciBuild: model.LuciBuild{
				Status: buildbucketpb.Status_STATUS_UNSPECIFIED,
			},
		}
		assert.Loosely(t, datastore.Put(c, rrBuild1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rerun1 := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, cfa1),
			RerunBuild: datastore.KeyForObj(c, rrBuild1),
			CreateTime: clock.Now(c).Add(-10 * time.Second),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		assert.Loosely(t, datastore.Put(c, rerun1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Create another rerun for chromeos
		cfa2 := createRunningAnalysis(c, t, 456, "chromeos", model.PlatformMac)

		rrBuild2 := &model.CompileRerunBuild{
			LuciBuild: model.LuciBuild{
				Status: buildbucketpb.Status_STARTED,
			},
		}
		assert.Loosely(t, datastore.Put(c, rrBuild2), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rerun2 := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, cfa2),
			RerunBuild: datastore.KeyForObj(c, rrBuild2),
			CreateTime: clock.Now(c).Add(time.Minute),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		assert.Loosely(t, datastore.Put(c, rerun2), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		err := collectMetricsForRunningReruns(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runningRerunGauge.Get(c, "chromium", "pending", "linux", "compile"), should.Equal(1))
		assert.Loosely(t, runningRerunGauge.Get(c, "chromium", "running", "linux", "compile"), should.BeZero)
		assert.Loosely(t, runningRerunGauge.Get(c, "chromeos", "pending", "mac", "compile"), should.BeZero)
		assert.Loosely(t, runningRerunGauge.Get(c, "chromeos", "running", "mac", "compile"), should.Equal(1))
		dist := rerunAgeMetric.Get(c, "chromium", "pending", "linux", "compile")
		assert.Loosely(t, dist.Count(), should.Equal(1))
		dist = rerunAgeMetric.Get(c, "chromeos", "running", "mac", "compile")
		assert.Loosely(t, dist.Count(), should.Equal(1))
	})

	ftt.Run("running test reruns", t, func(t *ftt.Test) {
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)
		createRerun := func(ID int64, project, OS string, status buildbucketpb.Status) {
			rerun := &model.TestSingleRerun{
				ID:     ID,
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				LUCIBuild: model.LUCIBuild{
					Project:    project,
					Status:     status,
					CreateTime: clock.Now(c).Add(-10 * time.Second),
				},
				Dimensions: &pb.Dimensions{Dimensions: []*pb.Dimension{{
					Key:   "os",
					Value: OS,
				}}},
			}
			assert.Loosely(t, datastore.Put(c, rerun), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
		}
		createRerun(100, "chromium", "Ubuntu-22.04", buildbucketpb.Status_SCHEDULED)
		createRerun(101, "chromium", "Ubuntu-22.04", buildbucketpb.Status_STARTED)
		createRerun(102, "chromium", "Ubuntu-22.04", buildbucketpb.Status_STARTED)
		createRerun(103, "chromium", "Mac-12", buildbucketpb.Status_STARTED)
		createRerun(104, "chromeos", "Ubuntu-22.04", buildbucketpb.Status_STARTED)

		err := collectMetricsForRunningTestReruns(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, runningRerunGauge.Get(c, "chromium", "running", "linux", "test"), should.Equal(2))
		assert.Loosely(t, runningRerunGauge.Get(c, "chromium", "pending", "linux", "test"), should.Equal(1))
		assert.Loosely(t, runningRerunGauge.Get(c, "chromium", "running", "mac", "test"), should.Equal(1))
		assert.Loosely(t, runningRerunGauge.Get(c, "chromeos", "running", "linux", "test"), should.Equal(1))
		dist := rerunAgeMetric.Get(c, "chromium", "pending", "linux", "test")
		assert.Loosely(t, dist.Count(), should.Equal(1))
		dist = rerunAgeMetric.Get(c, "chromeos", "running", "linux", "test")
		assert.Loosely(t, dist.Count(), should.Equal(1))
	})
}

func createRunningAnalysis(c context.Context, t testing.TB, id int64, proj string, platform model.Platform) *model.CompileFailureAnalysis {
	t.Helper()
	fb := &model.LuciFailedBuild{
		Id: id,
		LuciBuild: model.LuciBuild{
			Project: proj,
		},
		Platform: platform,
	}
	assert.Loosely(t, datastore.Put(c, fb), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()

	cf := testutil.CreateCompileFailure(c, t, fb)
	cfa := &model.CompileFailureAnalysis{
		Id:             id,
		CompileFailure: datastore.KeyForObj(c, cf),
		RunStatus:      pb.AnalysisRunStatus_STARTED,
	}
	assert.Loosely(t, datastore.Put(c, cfa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

type mockTreeStatusClient struct {
	getStatusResponse *tspb.Status
	getStatusErr      error
}

func (m *mockTreeStatusClient) GetStatus(ctx context.Context, in *tspb.GetStatusRequest, opts ...grpc.CallOption) (*tspb.Status, error) {
	return m.getStatusResponse, m.getStatusErr
}

func (m *mockTreeStatusClient) CreateStatus(ctx context.Context, in *tspb.CreateStatusRequest, opts ...grpc.CallOption) (*tspb.Status, error) {
	return nil, nil
}

func (m *mockTreeStatusClient) ListStatus(ctx context.Context, in *tspb.ListStatusRequest, opts ...grpc.CallOption) (*tspb.ListStatusResponse, error) {
	return nil, nil
}

func TestUpdateTreeMetrics(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	c, _ = tsmon.WithDummyInMemory(c)
	datastore.GetTestable(c).Consistent(true)

	ftt.Run("Tree is open", t, func(t *ftt.Test) {
		client := &mockTreeStatusClient{
			getStatusResponse: &tspb.Status{
				GeneralState: tspb.GeneralState_OPEN,
				CreateTime:   timestamppb.New(time.Unix(1000, 0)),
			},
		}
		err := updateTreeMetricsWithClient(c, client)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, treeIsOpen.Get(c, "chromium"), should.BeTrue)
		assert.Loosely(t, treeClosedDuration.Get(c, "chromium"), should.Equal(int64(0)))
	})

	ftt.Run("Tree is closed", t, func(t *ftt.Test) {
		ctx := c
		cl := testclock.New(testclock.TestTimeUTC)
		ctx = clock.Set(ctx, cl)
		cl.Set(time.Unix(2000, 0))

		client := &mockTreeStatusClient{
			getStatusResponse: &tspb.Status{
				GeneralState: tspb.GeneralState_CLOSED,
				CreateTime:   timestamppb.New(time.Unix(1000, 0)),
			},
		}
		err := updateTreeMetricsWithClient(ctx, client)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, treeIsOpen.Get(ctx, "chromium"), should.BeFalse)
		assert.Loosely(t, treeClosedDuration.Get(ctx, "chromium"), should.Equal(int64(1000)))
	})
}
