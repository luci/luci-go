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

package compilefailuredetection

import (
	"context"
	"testing"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestFailureDetection(t *testing.T) {
	t.Parallel()

	Convey("Has Compile Step Status", t, func() {
		c := context.Background()
		Convey("No Compile Step", func() {
			build := &buildbucketpb.Build{
				Steps: []*buildbucketpb.Step{},
			}
			So(hasCompileStepStatus(c, build, buildbucketpb.Status_FAILURE), ShouldBeFalse)
		})
		Convey("Has Compile Step", func() {
			build := &buildbucketpb.Build{
				Steps: []*buildbucketpb.Step{
					{
						Name:   "compile",
						Status: buildbucketpb.Status_FAILURE,
					},
				},
			}
			So(hasCompileStepStatus(c, build, buildbucketpb.Status_FAILURE), ShouldBeTrue)
			So(hasCompileStepStatus(c, build, buildbucketpb.Status_SUCCESS), ShouldBeFalse)
		})
	})

	Convey("Should analyze build", t, func() {
		build := &buildbucketpb.Build{}
		c := context.Background()
		So(shouldAnalyzeBuild(c, build), ShouldBeFalse)
		build.Status = buildbucketpb.Status_FAILURE
		So(shouldAnalyzeBuild(c, build), ShouldBeFalse)
		build.Steps = []*buildbucketpb.Step{
			{
				Name:   "compile",
				Status: buildbucketpb.Status_FAILURE,
			},
		}
		So(shouldAnalyzeBuild(c, build), ShouldBeTrue)
	})

	Convey("GetLastPassedFirstFailedBuild", t, func() {
		c := context.Background()
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx

		Convey("No builds", func() {
			res := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{},
			}
			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			_, _, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			So(err, ShouldNotBeNil)
		})

		Convey("Got succeeded builds", func() {
			res := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{
					{
						Id:     123,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     122,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     121,
						Status: buildbucketpb.Status_INFRA_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     120,
						Status: buildbucketpb.Status_INFRA_FAILURE,
					},
					{
						Id:     119,
						Status: buildbucketpb.Status_SUCCESS,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     118,
						Status: buildbucketpb.Status_SUCCESS,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
				},
			}
			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			So(err, ShouldBeNil)
			So(lastPassedBuild.Id, ShouldEqual, 118)
			So(firstFailedBuild.Id, ShouldEqual, 122)
		})

		Convey("Last passed build not in first search", func() {
			firstRes := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{
					{
						Id:     123,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     122,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
				},
				NextPageToken: "test-token",
			}
			secondRes := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{
					{
						Id:     121,
						Status: buildbucketpb.Status_INFRA_FAILURE,
					},
					{
						Id:     120,
						Status: buildbucketpb.Status_SUCCESS,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_SUCCESS,
							},
						},
					},
					{
						Id:     119,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
				},
			}

			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(firstRes, nil).Times(1)
			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(secondRes, nil).Times(1)
			lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			So(err, ShouldBeNil)
			So(lastPassedBuild.Id, ShouldEqual, 120)
			So(firstFailedBuild.Id, ShouldEqual, 122)
		})

		Convey("Fewer older builds than the search limit and all failed", func() {
			res := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{
					{
						Id:     123,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     122,
						Status: buildbucketpb.Status_FAILURE,
						Steps: []*buildbucketpb.Step{
							{
								Name:   "compile",
								Status: buildbucketpb.Status_FAILURE,
							},
						},
					},
					{
						Id:     121,
						Status: buildbucketpb.Status_INFRA_FAILURE,
					},
				},
			}
			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			So(lastPassedBuild, ShouldBeNil)
			So(firstFailedBuild, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("No recent passed build", func() {
			failedBuilds := make([]*buildbucketpb.Build, 100)
			for i := 0; i < 100; i++ {
				failedBuilds[i] = &buildbucketpb.Build{
					Id:     int64(123 - i),
					Status: buildbucketpb.Status_FAILURE,
					Steps: []*buildbucketpb.Step{
						{
							Name:   "compile",
							Status: buildbucketpb.Status_FAILURE,
						},
					},
				}
			}

			// Mock the return of older builds in batches, setting the response's
			// NextPageToken value to signify there are more results available
			for i := 0; i < 5; i++ {
				res := &buildbucketpb.SearchBuildsResponse{
					Builds:        failedBuilds[20*i : 20*(i+1)],
					NextPageToken: "test-token",
				}
				mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			}

			lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			So(lastPassedBuild, ShouldBeNil)
			So(firstFailedBuild, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("analysisExists", t, func() {
		c := memory.Use(context.Background())

		build := &buildbucketpb.Build{
			Id: 8002,
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "ios",
			},
			Number:     123,
			Status:     buildbucketpb.Status_FAILURE,
			StartTime:  &timestamppb.Timestamp{Seconds: 100},
			EndTime:    &timestamppb.Timestamp{Seconds: 101},
			CreateTime: &timestamppb.Timestamp{Seconds: 100},
			Input: &buildbucketpb.Build_Input{
				GitilesCommit: &buildbucketpb.GitilesCommit{},
			},
		}

		firstFailedBuild := &buildbucketpb.Build{
			Id: 8001,
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "ios",
			},
			Number:     122,
			Status:     buildbucketpb.Status_FAILURE,
			StartTime:  &timestamppb.Timestamp{Seconds: 100},
			EndTime:    &timestamppb.Timestamp{Seconds: 101},
			CreateTime: &timestamppb.Timestamp{Seconds: 100},
		}

		Convey("There is no existing analysis", func() {
			check, cf, e := analysisExists(c, build, firstFailedBuild)
			So(check, ShouldBeTrue)
			So(cf, ShouldNotBeNil)
			So(e, ShouldBeNil)
		})

		Convey("There is existing analysis", func() {
			failed_build := &model.LuciFailedBuild{
				Id: 8001,
				LuciBuild: model.LuciBuild{
					BuildId: 8001,
				},
				BuildFailureType: gfipb.BuildFailureType_COMPILE,
			}
			So(datastore.Put(c, failed_build), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			compile_failure := &model.CompileFailure{
				Id:    8001,
				Build: datastore.KeyForObj(c, failed_build),
			}
			So(datastore.Put(c, compile_failure), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			compile_failure_analysis := &model.CompileFailureAnalysis{
				CompileFailure:     datastore.KeyForObj(c, compile_failure),
				FirstFailedBuildId: 8001,
				LastPassedBuildId:  8000,
			}
			So(datastore.Put(c, compile_failure_analysis), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			check, cf, e := analysisExists(c, build, firstFailedBuild)
			So(check, ShouldBeFalse)
			So(e, ShouldBeNil)
			So(cf, ShouldNotBeNil)
			So(cf.Id, ShouldEqual, 8002)
			So(cf.MergedFailureKey.IntID(), ShouldEqual, 8001)
		})
	})

	Convey("createCompileFailureModel", t, func() {
		c := memory.Use(context.Background())

		build := &buildbucketpb.Build{
			Id: 8003,
			Builder: &buildbucketpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "ios",
			},
			Number:     124,
			Status:     buildbucketpb.Status_FAILURE,
			StartTime:  &timestamppb.Timestamp{Seconds: 100},
			EndTime:    &timestamppb.Timestamp{Seconds: 101},
			CreateTime: &timestamppb.Timestamp{Seconds: 100},
			Input: &buildbucketpb.Build_Input{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "refs/heads/gfiTest",
					Ref:     "1",
				},
			},
		}

		// Create a CompileFailure record in datastore
		compileFailure, err := createCompileFailureModel(c, build)
		So(compileFailure, ShouldNotBeNil)
		So(err, ShouldBeNil)

		Convey("Can create LuciFailedBuild with same info", func() {
			// Get the record from datastore
			failedBuild := &model.LuciFailedBuild{Id: 8003}
			err := datastore.Get(c, failedBuild)
			So(err, ShouldBeNil)
			// Check that the build information matches
			So(failedBuild, ShouldResemble, &model.LuciFailedBuild{
				Id: 8003,
				LuciBuild: model.LuciBuild{
					BuildId:     8003,
					Project:     "chromium",
					Bucket:      "ci",
					Builder:     "ios",
					BuildNumber: 124,
					GitilesCommit: buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "refs/heads/gfiTest",
						Ref:     "1",
					},
					CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
					EndTime:    (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
					StartTime:  (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
					Status:     buildbucketpb.Status_FAILURE,
				},
				BuildFailureType: gfipb.BuildFailureType_COMPILE,
			})
		})
	})
}
