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

// package culpritverification verifies if a suspect is a culprit.
package culpritverification

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
)

func TestVerifySuspect(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	// Setup mock for buildbucket
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx
	res1 := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "gofindit-single-revision",
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
			Builder: "gofindit-single-revision",
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

	Convey("Verify Suspect", t, func() {
		gitilesResponse := model.ChangeLogResponse{
			Log: []*model.ChangeLog{
				{
					Commit: "3424",
				},
			},
		}
		gitilesResponseStr, _ := json.Marshal(gitilesResponse)
		c = gitiles.MockedGitilesClientContext(c, map[string]string{
			"https://chromium.googlesource.com/chromium/src/+log/3425~2..3425^": string(gitilesResponseStr),
		})

		fb := &model.LuciFailedBuild{}
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:            111,
			Build:         datastore.KeyForObj(c, fb),
			OutputTargets: []string{"target1"},
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis := &model.CompileFailureAnalysis{
			Id:             444,
			CompileFailure: datastore.KeyForObj(c, compileFailure),
		}
		So(datastore.Put(c, analysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, heuristicAnalysis), ShouldBeNil)
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
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		err := VerifySuspect(c, suspect, 8000, 444)
		So(err, ShouldBeNil)
		So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_UnderVerification)
		datastore.GetTestable(c).CatchupIndexes()

		// Check that 2 rerun builds were created, and linked to suspect
		rerun1 := &model.CompileRerunBuild{
			Id: suspect.SuspectRerunBuild.IntID(),
		}
		err = datastore.Get(c, rerun1)
		So(err, ShouldBeNil)
		So(rerun1, ShouldResemble, &model.CompileRerunBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				BuildId:       123,
				Project:       "chromium",
				Bucket:        "findit",
				Builder:       "gofindit-single-revision",
				Status:        bbpb.Status_STARTED,
				GitilesCommit: *res1.Input.GitilesCommit,
				CreateTime:    res1.CreateTime.AsTime(),
				StartTime:     res1.StartTime.AsTime(),
			},
		})

		rerun2 := &model.CompileRerunBuild{
			Id: suspect.ParentRerunBuild.IntID(),
		}
		err = datastore.Get(c, rerun2)
		So(err, ShouldBeNil)
		So(rerun2, ShouldResemble, &model.CompileRerunBuild{
			Id: 456,
			LuciBuild: model.LuciBuild{
				BuildId:       456,
				Project:       "chromium",
				Bucket:        "findit",
				Builder:       "gofindit-single-revision",
				Status:        bbpb.Status_STARTED,
				GitilesCommit: *res2.Input.GitilesCommit,
				CreateTime:    res2.CreateTime.AsTime(),
				StartTime:     res2.StartTime.AsTime(),
			},
		})

		// Check that 2 SingleRerun model was created
		q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerun1))
		singleReruns := []*model.SingleRerun{}
		err = datastore.GetAll(c, q, &singleReruns)
		So(err, ShouldBeNil)
		So(len(singleReruns), ShouldEqual, 1)

		q = datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerun2))
		singleReruns = []*model.SingleRerun{}
		err = datastore.GetAll(c, q, &singleReruns)
		So(err, ShouldBeNil)
		So(len(singleReruns), ShouldEqual, 1)
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

	Convey("Has New Targets", t, func() {
		So(hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss.h"}, cls), ShouldBeTrue)
		So(hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_1.h"}, cls), ShouldBeTrue)
		So(hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_2.h"}, cls), ShouldBeTrue)
		So(hasNewTarget(c, []string{"device/bluetooth/floss/bluetooth_gatt_service_floss_3.h"}, cls), ShouldBeFalse)
	})
}

func TestGetPriority(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("GetPriority", t, func() {
		now := clock.Now(c)
		fb := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				StartTime: now,
				EndTime:   now.Add(9 * time.Minute),
			},
		}
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		So(datastore.Put(c, cf), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		ha := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		So(datastore.Put(c, ha), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, ha),
			Score:          1,
			Id:             123,
			ReviewUrl:      "reviewUrl",
		}
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err := getSuspectPriority(c, suspect)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 120)
		suspect.Score = 5
		pri, err = getSuspectPriority(c, suspect)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 100)
		suspect.Score = 15
		pri, err = getSuspectPriority(c, suspect)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 80)

		// Add another suspect
		suspect1 := &model.Suspect{
			Score:              1,
			Id:                 124,
			ReviewUrl:          "reviewUrl",
			VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
		}
		So(datastore.Put(c, suspect1), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getSuspectPriority(c, suspect)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 100)

		cfa.IsTreeCloser = true
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getSuspectPriority(c, suspect)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 30)
	})
}
