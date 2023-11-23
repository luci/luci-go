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

package bqexporter

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/model"
	bqpb "go.chromium.org/luci/bisection/proto/bq"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestExport(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	Convey("export", t, func() {
		// Create 5 test analyses.
		for i := 1; i <= 5; i++ {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				ID:         int64(1000 + i),
				RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
				Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_NOTFOUND),
				CreateTime: clock.Now(ctx).Add(time.Hour * time.Duration(-i)),
			})
			testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
				ID:        int64(2000 + i),
				Analysis:  tfa,
				IsPrimary: true,
			})
		}

		client := &fakeExportClient{}
		err := export(ctx, client)
		So(err, ShouldBeNil)
		// Filtered out 3.
		So(len(client.rows), ShouldEqual, 2)
		So(client.rows[0].AnalysisId, ShouldEqual, 1002)
		So(client.rows[1].AnalysisId, ShouldEqual, 1004)
	})
}
func TestFetchTestAnalyses(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(3600*24*30, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)

	Convey("fetch test analyses", t, func() {
		// Not ended, should be skipped.
		tf1 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1001,
			RunStatus:  bisectionpb.AnalysisRunStatus_STARTED,
			CreateTime: clock.Now(ctx).Add(-time.Hour),
		})
		// Ended, but from a long time ago. Should be skipped.
		tf2 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1002,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			CreateTime: clock.Now(ctx).Add(-15 * 24 * time.Hour),
		})
		// Ended, not found.
		tf3 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1003,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_NOTFOUND),
			CreateTime: clock.Now(ctx).Add(-time.Hour),
		})
		// Ended, found, but action not taken, ended recently, should be skipped.
		tf4 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1004,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-time.Hour),
			EndTime:    clock.Now(ctx).Add(-time.Minute),
		})
		createSuspect(ctx, tf4, false)
		// Ended, found, actions taken.
		tf5 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1005,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-2 * time.Hour),
		})
		createSuspect(ctx, tf5, true)
		// Ended, found, but action not taken, ended long time ago.
		tf6 := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:         1006,
			RunStatus:  bisectionpb.AnalysisRunStatus_ENDED,
			Status:     bisectionpb.AnalysisStatus(bisectionpb.AnalysisStatus_FOUND),
			CreateTime: clock.Now(ctx).Add(-26 * time.Hour),
			EndTime:    clock.Now(ctx).Add(-25 * time.Hour),
		})
		createSuspect(ctx, tf6, false)
		So(datastore.Put(ctx, []*model.TestFailureAnalysis{tf1, tf2, tf3, tf4, tf5, tf6}), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		tfas, err := fetchTestAnalyses(ctx)
		So(err, ShouldBeNil)
		So(len(tfas), ShouldEqual, 3)
		So(tfas[0].ID, ShouldEqual, 1003)
		So(tfas[1].ID, ShouldEqual, 1005)
		So(tfas[2].ID, ShouldEqual, 1006)
	})
}

func createSuspect(ctx context.Context, tfa *model.TestFailureAnalysis, hasTakenAction bool) {
	suspect := &model.Suspect{
		Id: tfa.ID,
		ActionDetails: model.ActionDetails{
			HasTakenActions: hasTakenAction,
		},
	}
	So(datastore.Put(ctx, suspect), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
}

// Fake client.
type fakeExportClient struct {
	rows []*bqpb.TestAnalysisRow
}

func (cl *fakeExportClient) EnsureSchema(ctx context.Context) error {
	return nil
}

func (cl *fakeExportClient) Insert(ctx context.Context, rows []*bqpb.TestAnalysisRow) error {
	cl.rows = append(cl.rows, rows...)
	return nil
}

func (cl *fakeExportClient) ReadTestFailureAnalysisRows(ctx context.Context) ([]*TestFailureAnalysisRow, error) {
	return []*TestFailureAnalysisRow{
		{
			AnalysisID: 1001,
		},
		{
			AnalysisID: 1003,
		},
		{
			AnalysisID: 1005,
		},
	}, nil
}
