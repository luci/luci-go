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

package rerun

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRerun(t *testing.T) {
	t.Parallel()

	Convey("getRerunPropertiesAndDimensions", t, func() {
		c := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		// Setup mock for buildbucket
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		bootstrapProperties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"bs_key_1": structpb.NewStringValue("bs_val_1"),
			},
		}

		targetBuilder := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"builder": structpb.NewStringValue("linux-test"),
				"group":   structpb.NewStringValue("buildergroup1"),
			},
		}

		res := &bbpb.Build{
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "linux-test",
			},
			Input: &bbpb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"builder_group":         structpb.NewStringValue("buildergroup1"),
						"$bootstrap/properties": structpb.NewStructValue(bootstrapProperties),
						"another_prop":          structpb.NewStringValue("another_val"),
					},
				},
			},
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{
					TaskDimensions: []*bbpb.RequestedDimension{
						{
							Key:   "dimen_key_1",
							Value: "dimen_val_1",
						},
						{
							Key:   "os",
							Value: "ubuntu",
						},
						{
							Key:   "gpu",
							Value: "Intel",
						},
					},
				},
			},
		}
		Convey("has extra prop and dim", func() {
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
			extraProps := map[string]any{
				"analysis_id":     4646418413256704,
				"compile_targets": []string{"target"},
				"bisection_host":  "luci-bisection.appspot.com",
			}
			extraDimens := map[string]string{
				"id": "bot-12345",
			}

			props, dimens, err := getRerunPropertiesAndDimensions(c, 1234, extraProps, extraDimens)
			So(err, ShouldBeNil)
			So(props, ShouldResemble, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"builder_group":  structpb.NewStringValue("buildergroup1"),
					"target_builder": structpb.NewStructValue(targetBuilder),
					"$bootstrap/properties": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"bs_key_1": structpb.NewStringValue("bs_val_1"),
						},
					}),
					"analysis_id":     structpb.NewNumberValue(4646418413256704),
					"compile_targets": structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("target")}}),
					"bisection_host":  structpb.NewStringValue("luci-bisection.appspot.com"),
				},
			})
			So(dimens, ShouldResemble, []*bbpb.RequestedDimension{
				{
					Key:   "os",
					Value: "ubuntu",
				},
				{
					Key:   "gpu",
					Value: "Intel",
				},
				{
					Key:   "id",
					Value: "bot-12345",
				},
			})
		})

		Convey("no extra dim", func() {
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

			_, dimens, err := getRerunPropertiesAndDimensions(c, 1234, nil, nil)
			So(err, ShouldBeNil)
			So(dimens, ShouldResemble, []*bbpb.RequestedDimension{
				{
					Key:   "os",
					Value: "ubuntu",
				},
				{
					Key:   "gpu",
					Value: "Intel",
				},
			})
		})

		Convey("builder is a tester", func() {
			res.Input.Properties.Fields["parent_build_id"] = structpb.NewStringValue("123")
			parentBuild := &bbpb.Build{
				Infra: &bbpb.BuildInfra{Swarming: &bbpb.BuildInfra_Swarming{
					TaskDimensions: []*bbpb.RequestedDimension{{Key: "os", Value: "parent os"}}},
				}}
			gomock.InOrder(
				mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil),
				mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(parentBuild, nil),
			)

			_, dimens, err := getRerunPropertiesAndDimensions(c, 1234, nil, nil)
			So(err, ShouldBeNil)
			So(dimens, ShouldResemble, []*bbpb.RequestedDimension{
				{
					Key:   "os",
					Value: "parent os",
				},
			})
		})
	})

}

func TestCreateRerunBuildModel(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	// Setup mock for buildbucket
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx
	res := &bbpb.Build{
		Infra: &bbpb.BuildInfra{
			Swarming: &bbpb.BuildInfra_Swarming{
				TaskDimensions: []*bbpb.RequestedDimension{
					{
						Key:   "dimen_key_1",
						Value: "dimen_val_1",
					},
					{
						Key:   "os",
						Value: "ubuntu",
					},
					{
						Key:   "gpu",
						Value: "Intel",
					},
				},
			},
		},
	}
	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

	build := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "luci-bisection-single-revision",
		},
		Input: &bbpb.Build_Input{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "3425",
			},
		},
		Id:         123,
		Status:     bbpb.Status_STARTED,
		CreateTime: &timestamppb.Timestamp{Seconds: 100},
		StartTime:  &timestamppb.Timestamp{Seconds: 101},
	}

	Convey("Create rerun build", t, func() {
		compileFailure := &model.CompileFailure{
			Id:            111,
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

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)
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
				Ref:     "ref",
				Id:      "3425",
			},
		}
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("Invalid data", func() {
			_, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, nil, nsa, 0)
			So(err, ShouldNotBeNil)
			_, err = CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, suspect, nil, 0)
			So(err, ShouldNotBeNil)
		})

		Convey("Culprit verification", func() {
			rerunBuildModel, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, suspect, nil, 100)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(rerunBuildModel, ShouldResemble, &model.CompileRerunBuild{
				Id: 123,
				LuciBuild: model.LuciBuild{
					BuildId: 123,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "luci-bisection-single-revision",
					Status:  bbpb.Status_STARTED,
					GitilesCommit: bbpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
						Id:      "3425",
					},
					CreateTime: build.CreateTime.AsTime(),
					StartTime:  build.StartTime.AsTime(),
				},
			})

			// Check SingleRerun
			q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerunBuildModel))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			So(err, ShouldBeNil)
			So(len(singleReruns), ShouldEqual, 1)
			So(singleReruns[0].Suspect, ShouldResemble, datastore.KeyForObj(c, suspect))
			So(singleReruns[0].Analysis, ShouldResemble, datastore.KeyForObj(c, analysis))
			So(singleReruns[0].Type, ShouldEqual, model.RerunBuildType_CulpritVerification)
			So(singleReruns[0].Dimensions, ShouldResembleProto, util.ToDimensionsPB(res.Infra.Swarming.TaskDimensions))
		})

		Convey("Nth Section", func() {
			build.Id = 124
			rerunBuildModel1, err := CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, nil, nsa, 100)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(rerunBuildModel1, ShouldResemble, &model.CompileRerunBuild{
				Id: 124,
				LuciBuild: model.LuciBuild{
					BuildId: 124,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "luci-bisection-single-revision",
					Status:  bbpb.Status_STARTED,
					GitilesCommit: bbpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
						Id:      "3425",
					},
					CreateTime: build.CreateTime.AsTime(),
					StartTime:  build.StartTime.AsTime(),
				},
			})

			// Check SingleRerun
			q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerunBuildModel1))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			So(err, ShouldBeNil)
			So(len(singleReruns), ShouldEqual, 1)
			So(singleReruns[0].NthSectionAnalysis, ShouldResemble, datastore.KeyForObj(c, nsa))
			So(singleReruns[0].Analysis, ShouldResemble, datastore.KeyForObj(c, analysis))
			So(singleReruns[0].Type, ShouldEqual, model.RerunBuildType_NthSection)
			So(singleReruns[0].Dimensions, ShouldResembleProto, util.ToDimensionsPB(res.Infra.Swarming.TaskDimensions))
		})
	})
}

func TestUpdateRerunStatus(t *testing.T) {
	t.Parallel()

	Convey("TestUpdateRerunStatus", t, func() {
		c := memory.Use(context.Background())
		testutil.UpdateIndices(c)

		// Setup mock for buildbucket
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		res := &bbpb.Build{
			Id: 1234,
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "findit",
				Builder: "luci-bisection-single-revision",
			},
			Status:    bbpb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 100},
			EndTime:   &timestamppb.Timestamp{Seconds: 200},
		}

		Convey("build starts", func() {
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
			rerunBuild := &model.CompileRerunBuild{
				Id: 1234,
			}
			So(datastore.Put(c, rerunBuild), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			singleRerun := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuild),
				Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}
			So(datastore.Put(c, singleRerun), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			So(UpdateCompileRerunStatus(c, 1234), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Checking the start time
			So(datastore.Get(c, rerunBuild), ShouldBeNil)
			So(rerunBuild.StartTime.Unix(), ShouldEqual, 100)
			So(rerunBuild.Status, ShouldEqual, bbpb.Status_STARTED)
			So(datastore.Get(c, singleRerun), ShouldBeNil)
			So(singleRerun.StartTime.Unix(), ShouldEqual, 100)
			So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_IN_PROGRESS)
		})

		Convey("build ends", func() {
			res.Status = bbpb.Status_SUCCESS
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
			Convey("rerun didn't end", func() {
				rerunBuild := &model.CompileRerunBuild{
					Id: 1234,
				}
				So(datastore.Put(c, rerunBuild), ShouldBeNil)
				singleRerun := &model.SingleRerun{
					RerunBuild: datastore.KeyForObj(c, rerunBuild),
					Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				}
				So(datastore.Put(c, singleRerun), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()

				So(UpdateCompileRerunStatus(c, 1234), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				So(datastore.Get(c, rerunBuild), ShouldBeNil)
				So(rerunBuild.EndTime.Unix(), ShouldEqual, 200)
				So(rerunBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
				So(datastore.Get(c, singleRerun), ShouldBeNil)
				So(singleRerun.EndTime.Unix(), ShouldEqual, 200)
				So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)
			})
			Convey("rerun ends", func() {
				rerunBuild := &model.CompileRerunBuild{
					Id: 1234,
				}
				So(datastore.Put(c, rerunBuild), ShouldBeNil)
				singleRerun := &model.SingleRerun{
					RerunBuild: datastore.KeyForObj(c, rerunBuild),
					Status:     pb.RerunStatus_RERUN_STATUS_PASSED,
					EndTime:    time.Unix(101, 0).UTC(),
				}
				So(datastore.Put(c, singleRerun), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()

				So(UpdateCompileRerunStatus(c, 1234), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				So(datastore.Get(c, rerunBuild), ShouldBeNil)
				So(rerunBuild.EndTime.Unix(), ShouldEqual, 200)
				So(rerunBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
				So(datastore.Get(c, singleRerun), ShouldBeNil)
				So(singleRerun.EndTime.Unix(), ShouldEqual, 101)
				So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_PASSED)
			})
		})
	})
}

func TestUpdateTestRerunStatus(t *testing.T) {
	t.Parallel()

	Convey("TestUpdateRerunStatus", t, func() {
		c := memory.Use(context.Background())
		testutil.UpdateIndices(c)
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		c = clock.Set(c, cl)

		build := &bbpb.Build{
			Id: 1234,
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "findit",
				Builder: "luci-bisection-single-revision",
			},
			Status:    bbpb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 100},
			EndTime:   &timestamppb.Timestamp{Seconds: 200},
		}

		Convey("build starts", func() {
			singleRerun := &model.TestSingleRerun{
				ID:     1234,
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}
			So(datastore.Put(c, singleRerun), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			So(UpdateTestRerunStatus(c, build), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Checking the start time
			So(datastore.Get(c, singleRerun), ShouldBeNil)
			So(singleRerun.LUCIBuild.StartTime.Unix(), ShouldEqual, 100)
			So(singleRerun.LUCIBuild.Status, ShouldEqual, bbpb.Status_STARTED)
		})

		Convey("build ends", func() {
			build.Status = bbpb.Status_SUCCESS
			Convey("rerun didn't end", func() {
				tfa := testutil.CreateTestFailureAnalysis(c, &testutil.TestFailureAnalysisCreationOption{
					ID: 100,
				})
				nsa := testutil.CreateTestNthSectionAnalysis(c, &testutil.TestNthSectionAnalysisCreationOption{
					ID:                1000,
					ParentAnalysisKey: datastore.KeyForObj(c, tfa),
				})
				singleRerun := testutil.CreateTestSingleRerun(c, &testutil.TestSingleRerunCreationOption{
					AnalysisKey: datastore.KeyForObj(c, tfa),
					ID:          1234,
				})
				So(datastore.Put(c, singleRerun), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()

				So(UpdateTestRerunStatus(c, build), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()

				// Checking the end time and status.
				So(datastore.Get(c, singleRerun), ShouldBeNil)
				So(singleRerun.LUCIBuild.EndTime.Unix(), ShouldEqual, 200)
				So(singleRerun.LUCIBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
				So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)

				So(datastore.Get(c, nsa), ShouldBeNil)
				So(nsa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
				So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
				So(nsa.EndTime.Unix(), ShouldEqual, 10000)

				So(datastore.Get(c, tfa), ShouldBeNil)
				So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
				So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
				So(tfa.EndTime.Unix(), ShouldEqual, 10000)
			})

			Convey("rerun ends", func() {
				singleRerun := &model.TestSingleRerun{
					ID:     1234,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				}
				So(datastore.Put(c, singleRerun), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()

				So(UpdateTestRerunStatus(c, build), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				So(datastore.Get(c, singleRerun), ShouldBeNil)
				So(singleRerun.LUCIBuild.EndTime.Unix(), ShouldEqual, 200)
				So(singleRerun.LUCIBuild.Status, ShouldEqual, bbpb.Status_SUCCESS)
				So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_PASSED)
			})
		})
	})
}
