// Copyright 2026 The LUCI Authors.
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

// Package cancelanalysis handles cancellation of existing test failure analyses.
package cancelanalysis

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestCancelAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx

	ftt.Run("Cancel Test Failure Analysis", t, func(t *ftt.Test) {
		t.Run("Cancel with confirmed culprit", func(t *ftt.Test) {
			tfa := &model.TestFailureAnalysis{
				ID:        123,
				Project:   "chromium",
				Status:    pb.AnalysisStatus_FOUND, // Culprit already confirmed
				RunStatus: pb.AnalysisRunStatus_ENDED,
			}
			assert.Loosely(t, datastore.Put(c, tfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.TestNthSectionAnalysis{
				ParentAnalysisKey: datastore.KeyForObj(c, tfa),
				Status:            pb.AnalysisStatus_RUNNING,
				RunStatus:         pb.AnalysisRunStatus_STARTED,
			}
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

			genaiAnalysis := &model.TestGenAIAnalysis{
				ParentAnalysisKey: datastore.KeyForObj(c, tfa),
				Status:            pb.AnalysisStatus_SUSPECTFOUND,
			}
			assert.Loosely(t, datastore.Put(c, genaiAnalysis), should.BeNil)

			suspect := &model.Suspect{
				ParentAnalysis:     datastore.KeyForObj(c, genaiAnalysis),
				Type:               model.SuspectType_GenAI,
				VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
				AnalysisType:       pb.AnalysisType_TEST_FAILURE_ANALYSIS,
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Create in-progress verification rerun for suspect
			rerun1 := &model.TestSingleRerun{
				ID: 1001,
				LUCIBuild: model.LUCIBuild{
					BuildID: 1001,
					Status:  bbpb.Status_STARTED,
				},
				Type:        model.RerunBuildType_CulpritVerification,
				AnalysisKey: datastore.KeyForObj(c, tfa),
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				CulpritKey:  datastore.KeyForObj(c, suspect),
			}

			// Create in-progress parent verification rerun
			rerun2 := &model.TestSingleRerun{
				ID: 1002,
				LUCIBuild: model.LUCIBuild{
					BuildID: 1002,
					Status:  bbpb.Status_STARTED,
				},
				Type:        model.RerunBuildType_CulpritVerification,
				AnalysisKey: datastore.KeyForObj(c, tfa),
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}

			// Create completed rerun (should not be canceled)
			rerun3 := &model.TestSingleRerun{
				ID: 1003,
				LUCIBuild: model.LUCIBuild{
					BuildID: 1003,
					Status:  bbpb.Status_SUCCESS,
				},
				Type:        model.RerunBuildType_NthSection,
				AnalysisKey: datastore.KeyForObj(c, tfa),
				Status:      pb.RerunStatus_RERUN_STATUS_PASSED,
			}

			// Create in-progress nth-section rerun
			rerun4 := &model.TestSingleRerun{
				ID: 1004,
				LUCIBuild: model.LUCIBuild{
					BuildID: 1004,
					Status:  bbpb.Status_STARTED,
				},
				Type:                  model.RerunBuildType_NthSection,
				AnalysisKey:           datastore.KeyForObj(c, tfa),
				Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
			}

			assert.Loosely(t, datastore.Put(c, rerun1), should.BeNil)
			assert.Loosely(t, datastore.Put(c, rerun2), should.BeNil)
			assert.Loosely(t, datastore.Put(c, rerun3), should.BeNil)
			assert.Loosely(t, datastore.Put(c, rerun4), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Expect 3 CancelBuild calls for in-progress reruns (rerun1, rerun2, rerun4)
			mc.Client.EXPECT().CancelBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(3)

			e := CancelAnalysis(c, 123)
			assert.Loosely(t, e, should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, tfa), should.BeNil)
			assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
			assert.Loosely(t, datastore.Get(c, rerun1), should.BeNil)
			assert.Loosely(t, datastore.Get(c, rerun2), should.BeNil)
			assert.Loosely(t, datastore.Get(c, rerun3), should.BeNil)
			assert.Loosely(t, datastore.Get(c, rerun4), should.BeNil)
			assert.Loosely(t, datastore.Get(c, suspect), should.BeNil)

			// Verify analysis statuses - should keep FOUND status since culprit was confirmed
			// RunStatus stays ENDED because UpdateAnalysisStatus doesn't update already-ended analyses
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))

			// Verify in-progress reruns were canceled
			assert.Loosely(t, rerun1.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
			assert.Loosely(t, rerun1.LUCIBuild.Status, should.Equal(bbpb.Status_CANCELED))
			assert.Loosely(t, rerun2.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
			assert.Loosely(t, rerun2.LUCIBuild.Status, should.Equal(bbpb.Status_CANCELED))
			assert.Loosely(t, rerun4.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
			assert.Loosely(t, rerun4.LUCIBuild.Status, should.Equal(bbpb.Status_CANCELED))

			// Verify completed rerun was NOT canceled
			assert.Loosely(t, rerun3.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))
			assert.Loosely(t, rerun3.LUCIBuild.Status, should.Equal(bbpb.Status_SUCCESS))

			// Verify suspect status was updated
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_Canceled))
		})

		t.Run("Cancel with no nth-section analysis", func(t *ftt.Test) {
			tfa := &model.TestFailureAnalysis{
				ID:        456,
				Project:   "chromium",
				Status:    pb.AnalysisStatus_FOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			}
			assert.Loosely(t, datastore.Put(c, tfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			rerun := &model.TestSingleRerun{
				ID: 2001,
				LUCIBuild: model.LUCIBuild{
					BuildID: 2001,
					Status:  bbpb.Status_STARTED,
				},
				Type:        model.RerunBuildType_CulpritVerification,
				AnalysisKey: datastore.KeyForObj(c, tfa),
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}
			assert.Loosely(t, datastore.Put(c, rerun), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			mc.Client.EXPECT().CancelBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(1)

			e := CancelAnalysis(c, 456)
			assert.Loosely(t, e, should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, tfa), should.BeNil)
			assert.Loosely(t, datastore.Get(c, rerun), should.BeNil)

			// Should succeed even without nth-section
			// RunStatus stays ENDED because UpdateAnalysisStatus doesn't update already-ended analyses
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_CANCELED))
		})

		t.Run("Cancel preserves already ended analysis", func(t *ftt.Test) {
			tfa := &model.TestFailureAnalysis{
				ID:        789,
				Project:   "chromium",
				Status:    pb.AnalysisStatus_SUSPECTFOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			}
			assert.Loosely(t, datastore.Put(c, tfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			e := CancelAnalysis(c, 789)
			assert.Loosely(t, e, should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, tfa), should.BeNil)

			// Analysis that has already ended should remain ENDED (not change to CANCELED)
			// This matches the behavior in UpdateAnalysisStatus which is a no-op for ended analyses
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})
	})
}
