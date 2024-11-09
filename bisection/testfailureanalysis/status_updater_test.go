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

package testfailureanalysis

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestUpdateAnalysisStatus(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateStatus", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		t.Run("Update status ended", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, tfa.EndTime.Unix(), should.Equal(10000))
		})

		t.Run("Update status started", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_CREATED,
				RunStatus: pb.AnalysisRunStatus_ANALYSIS_RUN_STATUS_UNSPECIFIED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_CREATED, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_CREATED))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
			assert.Loosely(t, tfa.StartTime.Unix(), should.Equal(10000))
		})

		t.Run("Do not update the start time of a started analysis", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
				StartTime: time.Unix(5000, 0).UTC(),
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
			assert.Loosely(t, tfa.StartTime.Unix(), should.Equal(5000))
		})

		t.Run("Ended analysis will not update", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_FOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})

		t.Run("Canceled analysis will not update", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_NOTFOUND,
				RunStatus: pb.AnalysisRunStatus_CANCELED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		})
	})
}

func TestUpdateNthSectionAnalysisStatus(t *testing.T) {
	t.Parallel()

	ftt.Run("Update Nthsection Status", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		t.Run("Update status ended", func(t *ftt.Test) {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, nsa.EndTime.Unix(), should.Equal(10000))
		})

		t.Run("Ended analysis will not update", func(t *ftt.Test) {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_FOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})

		t.Run("Canceled analysis will not update", func(t *ftt.Test) {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_NOTFOUND,
				RunStatus: pb.AnalysisRunStatus_CANCELED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		})
	})
}

func TestUpdateStatusWhenError(t *testing.T) {
	t.Parallel()

	ftt.Run("Update Status When Error", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			Status:    pb.AnalysisStatus_CREATED,
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			Status:            pb.AnalysisStatus_CREATED,
			RunStatus:         pb.AnalysisRunStatus_STARTED,
		})

		t.Run("No in progress rerun", func(t *ftt.Test) {
			err := UpdateAnalysisStatusWhenError(ctx, tfa)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))

			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, tfa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
		})

		t.Run("Have in progress rerun", func(t *ftt.Test) {
			testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			})
			err := UpdateAnalysisStatusWhenError(ctx, tfa)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
			// No change.
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_CREATED))
			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_CREATED))
		})
	})
}
