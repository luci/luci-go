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

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
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
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
		extraProps := map[string]interface{}{
			"analysis_id":     4646418413256704,
			"compile_targets": []string{"target"},
			"bisection_host":  "luci-bisection.appspot.com",
		}
		props, dimens, err := getRerunPropertiesAndDimensions(c, 1234, extraProps)
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
		})
	})
}

func TestCreateRerunBuildModel(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	build := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "gofindit-single-revision",
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
			_, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, nil, nsa)
			So(err, ShouldNotBeNil)
			_, err = CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, suspect, nil)
			So(err, ShouldNotBeNil)
		})

		Convey("Culprit verification", func() {
			rerunBuildModel, err := CreateRerunBuildModel(c, build, model.RerunBuildType_CulpritVerification, suspect, nil)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(rerunBuildModel, ShouldResemble, &model.CompileRerunBuild{
				Id: 123,
				LuciBuild: model.LuciBuild{
					BuildId: 123,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "gofindit-single-revision",
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
		})

		Convey("Nth Section", func() {
			build.Id = 124
			rerunBuildModel1, err := CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, nil, nsa)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(rerunBuildModel1, ShouldResemble, &model.CompileRerunBuild{
				Id: 124,
				LuciBuild: model.LuciBuild{
					BuildId: 124,
					Project: "chromium",
					Bucket:  "findit",
					Builder: "gofindit-single-revision",
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
		})

	})
}
