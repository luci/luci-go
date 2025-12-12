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

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

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
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestRerun(t *testing.T) {
	t.Parallel()

	ftt.Run("getRerunPropertiesAndDimensions", t, func(t *ftt.Test) {
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
		t.Run("has extra prop and dim", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, props, should.Match(&structpb.Struct{
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
			}))
			assert.Loosely(t, dimens, should.Match([]*bbpb.RequestedDimension{
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
			}))
		})

		t.Run("no extra dim", func(t *ftt.Test) {
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

			_, dimens, err := getRerunPropertiesAndDimensions(c, 1234, nil, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dimens, should.Match([]*bbpb.RequestedDimension{
				{
					Key:   "os",
					Value: "ubuntu",
				},
				{
					Key:   "gpu",
					Value: "Intel",
				},
			}))
		})

		t.Run("builder is a tester", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dimens, should.Match([]*bbpb.RequestedDimension{
				{
					Key:   "os",
					Value: "parent os",
				},
			}))
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

	ftt.Run("Create rerun build", t, func(t *ftt.Test) {
		compileFailure := &model.CompileFailure{
			Id:            111,
			OutputTargets: []string{"target1"},
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis := &model.CompileFailureAnalysis{
			Id:             444,
			CompileFailure: datastore.KeyForObj(c, compileFailure),
		}
		assert.Loosely(t, datastore.Put(c, analysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		genaiAnalysis := &model.CompileGenAIAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		assert.Loosely(t, datastore.Put(c, genaiAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			Score:          10,
			ParentAnalysis: datastore.KeyForObj(c, genaiAnalysis),
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "ref",
				Id:      "3425",
			},
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		t.Run("Invalid data", func(t *ftt.Test) {
			_, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, nil, nsa, 0)
			assert.Loosely(t, err, should.NotBeNil)
			_, err = CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, suspect, nil, 0)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Culprit verification", func(t *ftt.Test) {
			rerunBuildModel, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, suspect, nil, 100)
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerunBuildModel, should.Match(&model.CompileRerunBuild{
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
			}))

			// Check SingleRerun
			q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerunBuildModel))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.Equal(1))
			assert.Loosely(t, singleReruns[0].Suspect, should.Match(datastore.KeyForObj(c, suspect)))
			assert.Loosely(t, singleReruns[0].Analysis, should.Match(datastore.KeyForObj(c, analysis)))
			assert.Loosely(t, singleReruns[0].Type, should.Equal(model.RerunBuildType_CulpritVerification))
			assert.Loosely(t, singleReruns[0].Dimensions, should.Match(util.ToDimensionsPB(res.Infra.Swarming.TaskDimensions)))
		})

		t.Run("Nth Section", func(t *ftt.Test) {
			build.Id = 124
			rerunBuildModel1, err := CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, nil, nsa, 100)
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerunBuildModel1, should.Match(&model.CompileRerunBuild{
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
			}))

			// Check SingleRerun
			q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerunBuildModel1))
			singleReruns := []*model.SingleRerun{}
			err = datastore.GetAll(c, q, &singleReruns)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(singleReruns), should.Equal(1))
			assert.Loosely(t, singleReruns[0].NthSectionAnalysis, should.Match(datastore.KeyForObj(c, nsa)))
			assert.Loosely(t, singleReruns[0].Analysis, should.Match(datastore.KeyForObj(c, analysis)))
			assert.Loosely(t, singleReruns[0].Type, should.Equal(model.RerunBuildType_NthSection))
			assert.Loosely(t, singleReruns[0].Dimensions, should.Match(util.ToDimensionsPB(res.Infra.Swarming.TaskDimensions)))
		})
	})
}

func TestUpdateRerunStatus(t *testing.T) {
	t.Parallel()

	ftt.Run("TestUpdateRerunStatus", t, func(t *ftt.Test) {
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

		t.Run("build starts", func(t *ftt.Test) {
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
			rerunBuild := &model.CompileRerunBuild{
				Id: 1234,
			}
			assert.Loosely(t, datastore.Put(c, rerunBuild), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			singleRerun := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuild),
				Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}
			assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, UpdateCompileRerunStatus(c, 1234), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Checking the start time
			assert.Loosely(t, datastore.Get(c, rerunBuild), should.BeNil)
			assert.Loosely(t, rerunBuild.StartTime.Unix(), should.Equal(100))
			assert.Loosely(t, rerunBuild.Status, should.Equal(bbpb.Status_STARTED))
			assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
			assert.Loosely(t, singleRerun.StartTime.Unix(), should.Equal(100))
			assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_IN_PROGRESS))
		})

		t.Run("build ends", func(t *ftt.Test) {
			res.Status = bbpb.Status_SUCCESS
			mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
			t.Run("rerun didn't end", func(t *ftt.Test) {
				rerunBuild := &model.CompileRerunBuild{
					Id: 1234,
				}
				assert.Loosely(t, datastore.Put(c, rerunBuild), should.BeNil)
				singleRerun := &model.SingleRerun{
					RerunBuild: datastore.KeyForObj(c, rerunBuild),
					Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				}
				assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()

				assert.Loosely(t, UpdateCompileRerunStatus(c, 1234), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				assert.Loosely(t, datastore.Get(c, rerunBuild), should.BeNil)
				assert.Loosely(t, rerunBuild.EndTime.Unix(), should.Equal(200))
				assert.Loosely(t, rerunBuild.Status, should.Equal(bbpb.Status_SUCCESS))
				assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
				assert.Loosely(t, singleRerun.EndTime.Unix(), should.Equal(200))
				assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))
			})
			t.Run("rerun ends", func(t *ftt.Test) {
				rerunBuild := &model.CompileRerunBuild{
					Id: 1234,
				}
				assert.Loosely(t, datastore.Put(c, rerunBuild), should.BeNil)
				singleRerun := &model.SingleRerun{
					RerunBuild: datastore.KeyForObj(c, rerunBuild),
					Status:     pb.RerunStatus_RERUN_STATUS_PASSED,
					EndTime:    time.Unix(101, 0).UTC(),
				}
				assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()

				assert.Loosely(t, UpdateCompileRerunStatus(c, 1234), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				assert.Loosely(t, datastore.Get(c, rerunBuild), should.BeNil)
				assert.Loosely(t, rerunBuild.EndTime.Unix(), should.Equal(200))
				assert.Loosely(t, rerunBuild.Status, should.Equal(bbpb.Status_SUCCESS))
				assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
				assert.Loosely(t, singleRerun.EndTime.Unix(), should.Equal(101))
				assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))
			})
		})
	})
}

func TestCreateTestRerunModel(t *testing.T) {
	t.Parallel()

	ftt.Run("CreateTestRerunModel", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		testutil.UpdateIndices(c)
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
							Key:   "os",
							Value: "Ubuntu-22.04",
						},
						{
							Key:   "pool",
							Value: "luci.chromium.findit",
						},
					},
				},
			},
		}
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

		build := &bbpb.Build{
			Id:     123456,
			Number: 100,
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "findit",
				Builder: "test-single-revision",
			},
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "abc123def456",
				},
			},
			Status:     bbpb.Status_SCHEDULED,
			CreateTime: &timestamppb.Timestamp{Seconds: 1000},
			StartTime:  &timestamppb.Timestamp{Seconds: 1100},
		}

		// Create test failure analysis
		tfa := testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
			ID:       100,
			Project:  "chromium",
			Priority: 50,
		})

		// Create test failures
		tf1 := testutil.CreateTestFailure(c, t, &testutil.TestFailureCreationOption{
			ID:        200,
			Analysis:  tfa,
			IsPrimary: true,
		})
		tf2 := testutil.CreateTestFailure(c, t, &testutil.TestFailureCreationOption{
			ID:        201,
			Analysis:  tfa,
			IsPrimary: false,
		})

		t.Run("NthSection rerun", func(t *ftt.Test) {
			nsa := testutil.CreateTestNthSectionAnalysis(c, t, &testutil.TestNthSectionAnalysisCreationOption{
				ID:                1000,
				ParentAnalysisKey: datastore.KeyForObj(c, tfa),
			})

			options := CreateTestRerunModelOptions{
				TestFailureAnalysis:   tfa,
				NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
				TestFailures:          []*model.TestFailure{tf1, tf2},
				Build:                 build,
				RerunType:             model.RerunBuildType_NthSection,
			}

			rerun, err := CreateTestRerunModel(c, options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerun, should.NotBeNil)

			// Compare the entire struct using should.Match for better maintainability
			expected := &model.TestSingleRerun{
				ID: 123456,
				LUCIBuild: model.LUCIBuild{
					BuildID:     123456,
					Project:     "chromium",
					Bucket:      "findit",
					Builder:     "test-single-revision",
					BuildNumber: 100,
					GitilesCommit: &bbpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "abc123def456",
						Ref:     "refs/heads/main",
					},
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: time.Unix(1000, 0),
					StartTime:  time.Unix(1100, 0),
				},
				Type:                  model.RerunBuildType_NthSection,
				AnalysisKey:           datastore.KeyForObj(c, tfa),
				NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
				CulpritKey:            nil,
				Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Priority:              int32(50),
				TestResults: model.RerunTestResults{
					Results: []model.RerunSingleTestResult{
						{TestFailureKey: datastore.KeyForObj(c, tf1)},
						{TestFailureKey: datastore.KeyForObj(c, tf2)},
					},
				},
				Dimensions: &pb.Dimensions{
					Dimensions: []*pb.Dimension{
						{Key: "os", Value: "Ubuntu-22.04"},
						{Key: "pool", Value: "luci.chromium.findit"},
					},
				},
			}
			assert.Loosely(t, rerun, should.Match(expected))
		})

		t.Run("Culprit verification rerun", func(t *ftt.Test) {
			nsa := testutil.CreateTestNthSectionAnalysis(c, t, &testutil.TestNthSectionAnalysisCreationOption{
				ID:                1001,
				ParentAnalysisKey: datastore.KeyForObj(c, tfa),
			})

			suspect := &model.Suspect{
				Type: model.SuspectType_NthSection,
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "def456abc789",
				},
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				VerificationStatus: model.SuspectVerificationStatus_Unverified,
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			build.Id = 123457

			options := CreateTestRerunModelOptions{
				TestFailureAnalysis: tfa,
				SuspectKey:          datastore.KeyForObj(c, suspect),
				TestFailures:        []*model.TestFailure{tf1, tf2},
				Build:               build,
				RerunType:           model.RerunBuildType_CulpritVerification,
			}

			rerun, err := CreateTestRerunModel(c, options)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rerun, should.NotBeNil)

			// Compare the entire struct using should.Match for better maintainability
			expected := &model.TestSingleRerun{
				ID: 123457,
				LUCIBuild: model.LUCIBuild{
					BuildID:     123457,
					Project:     "chromium",
					Bucket:      "findit",
					Builder:     "test-single-revision",
					BuildNumber: 100,
					GitilesCommit: &bbpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "abc123def456",
						Ref:     "refs/heads/main",
					},
					Status:     bbpb.Status_SCHEDULED,
					CreateTime: time.Unix(1000, 0),
					StartTime:  time.Unix(1100, 0),
				},
				Type:                  model.RerunBuildType_CulpritVerification,
				AnalysisKey:           datastore.KeyForObj(c, tfa),
				NthSectionAnalysisKey: nil,
				CulpritKey:            datastore.KeyForObj(c, suspect),
				Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Priority:              int32(50),
				TestResults: model.RerunTestResults{
					Results: []model.RerunSingleTestResult{
						{TestFailureKey: datastore.KeyForObj(c, tf1)},
						{TestFailureKey: datastore.KeyForObj(c, tf2)},
					},
				},
				Dimensions: &pb.Dimensions{
					Dimensions: []*pb.Dimension{
						{Key: "os", Value: "Ubuntu-22.04"},
						{Key: "pool", Value: "luci.chromium.findit"},
					},
				},
			}
			assert.Loosely(t, rerun, should.Match(expected))
		})
	})
}

func TestUpdateTestRerunStatus(t *testing.T) {
	t.Parallel()

	ftt.Run("TestUpdateRerunStatus", t, func(t *ftt.Test) {
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

		t.Run("build starts", func(t *ftt.Test) {
			singleRerun := &model.TestSingleRerun{
				ID:     1234,
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			}
			assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, UpdateTestRerunStatus(c, build), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Checking the start time
			assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
			assert.Loosely(t, singleRerun.LUCIBuild.StartTime.Unix(), should.Equal(100))
			assert.Loosely(t, singleRerun.LUCIBuild.Status, should.Equal(bbpb.Status_STARTED))
		})

		t.Run("build ends", func(t *ftt.Test) {
			build.Status = bbpb.Status_SUCCESS
			t.Run("rerun didn't end", func(t *ftt.Test) {
				tfa := testutil.CreateTestFailureAnalysis(c, t, &testutil.TestFailureAnalysisCreationOption{
					ID: 100,
				})
				nsa := testutil.CreateTestNthSectionAnalysis(c, t, &testutil.TestNthSectionAnalysisCreationOption{
					ID:                1000,
					ParentAnalysisKey: datastore.KeyForObj(c, tfa),
				})
				singleRerun := testutil.CreateTestSingleRerun(c, t, &testutil.TestSingleRerunCreationOption{
					AnalysisKey: datastore.KeyForObj(c, tfa),
					ID:          1234,
				})
				assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()

				assert.Loosely(t, UpdateTestRerunStatus(c, build), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()

				// Checking the end time and status.
				assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
				assert.Loosely(t, singleRerun.LUCIBuild.EndTime.Unix(), should.Equal(200))
				assert.Loosely(t, singleRerun.LUCIBuild.Status, should.Equal(bbpb.Status_SUCCESS))
				assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))

				assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
				assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_ERROR))
				assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
				assert.Loosely(t, nsa.EndTime.Unix(), should.Equal(10000))

				assert.Loosely(t, datastore.Get(c, tfa), should.BeNil)
				assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
				assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
				assert.Loosely(t, tfa.EndTime.Unix(), should.Equal(10000))
			})

			t.Run("rerun ends", func(t *ftt.Test) {
				singleRerun := &model.TestSingleRerun{
					ID:     1234,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				}
				assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()

				assert.Loosely(t, UpdateTestRerunStatus(c, build), should.BeNil)
				datastore.GetTestable(c).CatchupIndexes()
				// Checking the end time and status.
				assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
				assert.Loosely(t, singleRerun.LUCIBuild.EndTime.Unix(), should.Equal(200))
				assert.Loosely(t, singleRerun.LUCIBuild.Status, should.Equal(bbpb.Status_SUCCESS))
				assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))
			})
		})
	})
}
