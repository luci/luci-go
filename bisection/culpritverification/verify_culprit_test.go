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

package culpritverification

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestVerifySuspect(t *testing.T) {
	t.Parallel()

	ftt.Run("Verify Suspect", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		testutil.UpdateIndices(c)

		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		c = hosts.UseHosts(c, hosts.ModuleOptions{
			APIHost: "test-bisection-host",
		})

		// Setup config.
		projectCfg := config.CreatePlaceholderProjectConfig()
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		assert.Loosely(t, config.SetTestProjectConfig(c, cfg), should.BeNil)

		t.Run("Verify Suspect triggers rerun", func(t *ftt.Test) {
			// Setup mock for buildbucket
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mc := buildbucket.NewMockedClient(c, ctl)
			c = mc.Ctx
			res1 := &bbpb.Build{
				Builder: &bbpb.BuilderID{
					Project: "chromium",
					Bucket:  "findit",
					Builder: "single-revision",
				},
				Input: &bbpb.Build_Input{
					GitilesCommit: &bbpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Id:      "id1",
						Ref:     "ref",
					},
				},
				Id:         123,
				Status:     bbpb.Status_STARTED,
				CreateTime: &timestamppb.Timestamp{Seconds: 100},
				StartTime:  &timestamppb.Timestamp{Seconds: 101},
			}
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res1, nil).Times(1)

			res2 := &bbpb.Build{
				Builder: &bbpb.BuilderID{
					Project: "chromium",
					Bucket:  "findit",
					Builder: "single-revision",
				},
				Input: &bbpb.Build_Input{
					GitilesCommit: &bbpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Id:      "id2",
						Ref:     "ref",
					},
				},
				Id:         456,
				Status:     bbpb.Status_STARTED,
				CreateTime: &timestamppb.Timestamp{Seconds: 200},
				StartTime:  &timestamppb.Timestamp{Seconds: 201},
			}
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res2, nil).Times(1)
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).AnyTimes()
			gitilesResponse := model.ChangeLogResponse{
				Log: []*model.ChangeLog{
					{
						Commit: "3424",
					},
				},
			}

			// Set up gitiles.
			gitilesResponseStr, _ := json.Marshal(gitilesResponse)
			c = gitiles.MockedGitilesClientContext(c, map[string]string{
				"https://chromium.googlesource.com/chromium/src/+log/3425~2..3425^": string(gitilesResponseStr),
			})

			fb := &model.LuciFailedBuild{}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			compileFailure := &model.CompileFailure{
				Id:            111,
				Build:         datastore.KeyForObj(c, fb),
				OutputTargets: []string{"target1"},
			}
			assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			analysis := &model.CompileFailureAnalysis{
				Id:                 444,
				CompileFailure:     datastore.KeyForObj(c, compileFailure),
				FirstFailedBuildId: 1000,
			}
			assert.Loosely(t, datastore.Put(c, analysis), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			heuristicAnalysis := &model.CompileHeuristicAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, analysis),
			}
			assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			suspect := &model.Suspect{
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "3425",
				},
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			err := processCulpritVerificationTask(c, 444, suspect.Id, suspect.ParentAnalysis.Encode())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(c, suspect), should.BeNil)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_UnderVerification))
			datastore.GetTestable(c).CatchupIndexes()

			// Check that 2 rerun builds were created, and linked to suspect
			rerun1 := &model.CompileRerunBuild{
				Id: suspect.SuspectRerunBuild.IntID(),
			}
			err = datastore.Get(c, rerun1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerun1, should.Resemble(&model.CompileRerunBuild{
				Id: 123,
				LuciBuild: model.LuciBuild{
					BuildId: 123,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "single-revision",
					Status:  bbpb.Status_STARTED,
					GitilesCommit: bbpb.GitilesCommit{
						Host:    res1.Input.GitilesCommit.Host,
						Project: res1.Input.GitilesCommit.Project,
						Id:      res1.Input.GitilesCommit.Id,
						Ref:     res1.Input.GitilesCommit.Ref,
					},
					CreateTime: res1.CreateTime.AsTime(),
					StartTime:  res1.StartTime.AsTime(),
				},
			}))

			rerun2 := &model.CompileRerunBuild{
				Id: suspect.ParentRerunBuild.IntID(),
			}
			err = datastore.Get(c, rerun2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerun2, should.Resemble(&model.CompileRerunBuild{
				Id: 456,
				LuciBuild: model.LuciBuild{
					BuildId: 456,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "single-revision",
					Status:  bbpb.Status_STARTED,
					GitilesCommit: bbpb.GitilesCommit{
						Host:    res2.Input.GitilesCommit.Host,
						Project: res2.Input.GitilesCommit.Project,
						Id:      res2.Input.GitilesCommit.Id,
						Ref:     res2.Input.GitilesCommit.Ref,
					},
					CreateTime: res2.CreateTime.AsTime(),
					StartTime:  res2.StartTime.AsTime(),
				},
			}))

			// Check that 2 SingleRerun model was created
			q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerun1))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.Equal(1))

			q = datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerun2))
			singleReruns = []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.Equal(1))
		})

		t.Run("Verify Suspect should not trigger any rerun if culprit found", func(t *ftt.Test) {
			_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(c, t, 8001, "chromium", 555)

			suspect := &model.Suspect{
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			cfa.VerifiedCulprits = []*datastore.Key{
				datastore.KeyForObj(c, suspect),
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			err := VerifySuspect(c, suspect, 8001, 555)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Check that no rerun was created
			q := datastore.NewQuery("SingleRerun").Eq("analysis", datastore.KeyForObj(c, cfa))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.BeZero)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))
		})

		t.Run("Verify Suspect should also update analysis status", func(t *ftt.Test) {
			_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(c, t, 8001, "chromium", 666)
			cfa.Status = pb.AnalysisStatus_SUSPECTFOUND
			cfa.RunStatus = pb.AnalysisRunStatus_STARTED
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := testutil.CreateNthSectionAnalysis(c, t, cfa)
			nsa.Status = pb.AnalysisStatus_SUSPECTFOUND
			nsa.RunStatus = pb.AnalysisRunStatus_ENDED
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			suspect := &model.Suspect{
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
					Id:      "id",
				},
			}
			// Create another suspect with same gitiles commit, so that no rerun build
			// is triggered
			suspect1 := &model.Suspect{
				VerificationStatus: model.SuspectVerificationStatus_Vindicated,
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
					Id:      "id",
				},
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			err := VerifySuspect(c, suspect, 8001, 666)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Check that no rerun was created
			q := datastore.NewQuery("SingleRerun").Eq("analysis", datastore.KeyForObj(c, cfa))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.BeZero)
			assert.Loosely(t, datastore.Get(c, suspect), should.BeNil)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))

			// Verify the status is updated
			assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})
	})
}

func TestHasNewTargets(t *testing.T) {
	cls := &model.ChangeLog{
		ChangeLogDiffs: []model.ChangeLogDiff{
			{
				Type:    model.ChangeType_ADD,
				NewPath: "src/device/bluetooth/floss/bluetooth_gatt_service_floss.h",
			},
			{
				Type:    model.ChangeType_RENAME,
				NewPath: "src/device/bluetooth/floss/bluetooth_gatt_service_floss_1.h",
			},
			{
				Type:    model.ChangeType_COPY,
				NewPath: "src/device/bluetooth/floss/bluetooth_gatt_service_floss_2.h",
			},
			{
				Type:    model.ChangeType_MODIFY,
				NewPath: "src/device/bluetooth/floss/bluetooth_gatt_service_floss_3.h",
			},
		},
	}

	c := context.Background()

	ftt.Run("Has New Targets", t, func(t *ftt.Test) {
		assert.Loosely(t, hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss.h"}, cls), should.BeTrue)
		assert.Loosely(t, hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_1.h"}, cls), should.BeTrue)
		assert.Loosely(t, hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_2.h"}, cls), should.BeTrue)
		assert.Loosely(t, hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_3.h"}, cls), should.BeFalse)
	})
}

func TestGetPriority(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("GetPriority", t, func(t *ftt.Test) {
		now := clock.Now(c)
		fb := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				StartTime: now,
				EndTime:   now.Add(9 * time.Minute),
			},
		}
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		ha := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		assert.Loosely(t, datastore.Put(c, ha), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, ha),
			Score:          1,
			Id:             123,
			ReviewUrl:      "reviewUrl",
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err := getSuspectPriority(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(120))
		suspect.Score = 5
		pri, err = getSuspectPriority(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(100))
		suspect.Score = 15
		pri, err = getSuspectPriority(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(80))

		// Add another suspect
		suspect1 := &model.Suspect{
			Score:              1,
			Id:                 124,
			ReviewUrl:          "reviewUrl",
			VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
		}
		assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getSuspectPriority(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(100))

		cfa.IsTreeCloser = true
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getSuspectPriority(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(30))
	})
}

func TestCheckSuspectWithSameCommitExist(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("CheckSuspectWithSameCommitExist", t, func(t *ftt.Test) {
		_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(c, t, 8000, "chromium", 555)
		nsa := testutil.CreateNthSectionAnalysis(c, t, cfa)
		suspect := testutil.CreateNthSectionSuspect(c, t, nsa)

		exist, err := checkSuspectWithSameCommitExist(c, cfa, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exist, should.BeFalse)

		ha := testutil.CreateHeuristicAnalysis(c, t, cfa)
		s1 := testutil.CreateHeuristicSuspect(c, t, ha, model.SuspectVerificationStatus_Unverified)

		exist, err = checkSuspectWithSameCommitExist(c, cfa, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exist, should.BeFalse)

		s1.VerificationStatus = model.SuspectVerificationStatus_UnderVerification
		assert.Loosely(t, datastore.Put(c, s1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		exist, err = checkSuspectWithSameCommitExist(c, cfa, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exist, should.BeTrue)
	})
}
