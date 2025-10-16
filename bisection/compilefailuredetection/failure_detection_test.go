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

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailureanalysis/cancelanalysis"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestFailureDetection(t *testing.T) {
	t.Parallel()

	ftt.Run("Has Build Step Status", t, func(t *ftt.Test) {
		c := context.Background()
		t.Run("No Build Step", func(t *ftt.Test) {
			build := &buildbucketpb.Build{
				Steps: []*buildbucketpb.Step{},
			}
			assert.Loosely(t, hasBuildStepStatus(c, build, buildbucketpb.Status_FAILURE), should.BeFalse)
		})
		t.Run("Has compile Step", func(t *ftt.Test) {
			build := &buildbucketpb.Build{
				Steps: []*buildbucketpb.Step{
					{
						Name:   "compile",
						Status: buildbucketpb.Status_FAILURE,
					},
				},
			}
			assert.Loosely(t, hasBuildStepStatus(c, build, buildbucketpb.Status_FAILURE), should.BeTrue)
			assert.Loosely(t, hasBuildStepStatus(c, build, buildbucketpb.Status_SUCCESS), should.BeFalse)
		})
		t.Run("Has generate_build_files Step", func(t *ftt.Test) {
			build := &buildbucketpb.Build{
				Steps: []*buildbucketpb.Step{
					{
						Name:   "generate_build_files",
						Status: buildbucketpb.Status_FAILURE,
					},
				},
			}
			assert.Loosely(t, hasBuildStepStatus(c, build, buildbucketpb.Status_FAILURE), should.BeTrue)
			assert.Loosely(t, hasBuildStepStatus(c, build, buildbucketpb.Status_SUCCESS), should.BeFalse)
		})
	})

	ftt.Run("Should analyze build", t, func(t *ftt.Test) {
		build := &buildbucketpb.Build{}
		c := context.Background()
		assert.Loosely(t, shouldAnalyzeBuild(c, build), should.BeFalse)
		build.Status = buildbucketpb.Status_FAILURE
		assert.Loosely(t, shouldAnalyzeBuild(c, build), should.BeFalse)
		build.Steps = []*buildbucketpb.Step{
			{
				Name:   "compile",
				Status: buildbucketpb.Status_FAILURE,
			},
		}
		assert.Loosely(t, shouldAnalyzeBuild(c, build), should.BeTrue)
	})

	ftt.Run("GetLastPassedFirstFailedBuild", t, func(t *ftt.Test) {
		c := context.Background()
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx

		t.Run("No builds", func(t *ftt.Test) {
			res := &buildbucketpb.SearchBuildsResponse{
				Builds: []*buildbucketpb.Build{},
			}
			mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			_, _, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Got succeeded builds", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lastPassedBuild.Id, should.Equal(118))
			assert.Loosely(t, firstFailedBuild.Id, should.Equal(122))
		})

		t.Run("Last passed build not in first search", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, lastPassedBuild.Id, should.Equal(120))
			assert.Loosely(t, firstFailedBuild.Id, should.Equal(122))
		})

		t.Run("Fewer older builds than the search limit and all failed", func(t *ftt.Test) {
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
			assert.Loosely(t, lastPassedBuild, should.BeNil)
			assert.Loosely(t, firstFailedBuild, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("No recent passed build", func(t *ftt.Test) {
			failedBuilds := make([]*buildbucketpb.Build, 100)
			for i := range 100 {
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
			for i := range 5 {
				res := &buildbucketpb.SearchBuildsResponse{
					Builds:        failedBuilds[20*i : 20*(i+1)],
					NextPageToken: "test-token",
				}
				mc.Client.EXPECT().SearchBuilds(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).Times(1)
			}

			lastPassedBuild, firstFailedBuild, err := getLastPassedFirstFailedBuilds(c, &buildbucketpb.Build{Id: 123})
			assert.Loosely(t, lastPassedBuild, should.BeNil)
			assert.Loosely(t, firstFailedBuild, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})

	ftt.Run("analysisExists", t, func(t *ftt.Test) {
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

		t.Run("There is no existing analysis", func(t *ftt.Test) {
			check, cf, e := analysisExists(c, build, firstFailedBuild)
			assert.Loosely(t, check, should.BeTrue)
			assert.Loosely(t, cf, should.NotBeNil)
			assert.Loosely(t, e, should.BeNil)
		})

		t.Run("There is existing analysis", func(t *ftt.Test) {
			failed_build := &model.LuciFailedBuild{
				Id: 8001,
				LuciBuild: model.LuciBuild{
					BuildId: 8001,
				},
				BuildFailureType: pb.BuildFailureType_COMPILE,
			}
			assert.Loosely(t, datastore.Put(c, failed_build), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			compile_failure := &model.CompileFailure{
				Id:    8001,
				Build: datastore.KeyForObj(c, failed_build),
			}
			assert.Loosely(t, datastore.Put(c, compile_failure), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			compile_failure_analysis := &model.CompileFailureAnalysis{
				CompileFailure:     datastore.KeyForObj(c, compile_failure),
				FirstFailedBuildId: 8001,
				LastPassedBuildId:  8000,
			}
			assert.Loosely(t, datastore.Put(c, compile_failure_analysis), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			check, cf, e := analysisExists(c, build, firstFailedBuild)
			assert.Loosely(t, check, should.BeFalse)
			assert.Loosely(t, e, should.BeNil)
			assert.Loosely(t, cf, should.NotBeNil)
			assert.Loosely(t, cf.Id, should.Equal(8002))
			assert.Loosely(t, cf.MergedFailureKey.IntID(), should.Equal(8001))
		})
	})

	ftt.Run("createCompileFailureModel", t, func(t *ftt.Test) {
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
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"sheriff_rotations": structpb.NewListValue(
							&structpb.ListValue{
								Values: []*structpb.Value{
									structpb.NewStringValue("chromium"),
								},
							},
						),
					},
				},
			},
			Output: &buildbucketpb.Build_Output{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Host:     "chromium.googlesource.com",
					Project:  "chromium/src",
					Id:       "refs/heads/gfiTest",
					Ref:      "1",
					Position: 456,
				},
			},
			Infra: &buildbucketpb.BuildInfra{
				Swarming: &buildbucketpb.BuildInfra_Swarming{
					TaskDimensions: []*buildbucketpb.RequestedDimension{
						{
							Key:   "os",
							Value: "Mac-12.4",
						},
					},
				},
			},
		}

		// Create a CompileFailure record in datastore
		compileFailure, err := createCompileFailureModel(c, build)
		assert.Loosely(t, compileFailure, should.NotBeNil)
		assert.Loosely(t, err, should.BeNil)

		t.Run("Can create LuciFailedBuild with same info", func(t *ftt.Test) {
			// Get the record from datastore
			failedBuild := &model.LuciFailedBuild{Id: 8003}
			err := datastore.Get(c, failedBuild)
			assert.Loosely(t, err, should.BeNil)
			// Check that the build information matches
			assert.Loosely(t, failedBuild, should.Match(&model.LuciFailedBuild{
				Id: 8003,
				LuciBuild: model.LuciBuild{
					BuildId:     8003,
					Project:     "chromium",
					Bucket:      "ci",
					Builder:     "ios",
					BuildNumber: 124,
					GitilesCommit: buildbucketpb.GitilesCommit{
						Host:     "chromium.googlesource.com",
						Project:  "chromium/src",
						Id:       "refs/heads/gfiTest",
						Ref:      "1",
						Position: 456,
					},
					CreateTime: (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
					EndTime:    (&timestamppb.Timestamp{Seconds: 101}).AsTime(),
					StartTime:  (&timestamppb.Timestamp{Seconds: 100}).AsTime(),
					Status:     buildbucketpb.Status_FAILURE,
				},
				BuildFailureType: pb.BuildFailureType_COMPILE,
				Platform:         model.PlatformMac,
				SheriffRotations: []string{"chromium"},
			}))
		})
	})
}

func TestUpdateSucceededBuild(t *testing.T) {
	t.Parallel()

	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(c, ctl)
	c = mc.Ctx

	res := &buildbucketpb.Build{
		Id: 123,
		Builder: &buildbucketpb.BuilderID{
			Project: "project",
			Bucket:  "bucket",
			Builder: "builder",
		},
		Number: 13,
	}
	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()

	ftt.Run("UpdateSucceededBuild no build", t, func(t *ftt.Test) {
		err := UpdateSucceededBuild(c, 123)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("UpdateSucceededBuild", t, func(t *ftt.Test) {
		c, scheduler := tq.TestingContext(c, nil)
		cancelanalysis.RegisterTaskClass()

		bf := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				Project:     "project",
				Bucket:      "bucket",
				Builder:     "builder",
				EndTime:     clock.Now(c),
				BuildNumber: 12,
			},
		}

		assert.Loosely(t, datastore.Put(c, bf), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, bf),
		}
		assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			Id:             789,
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		err := UpdateSucceededBuild(c, 123)
		datastore.GetTestable(c).CatchupIndexes()

		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
		assert.Loosely(t, cfa.ShouldCancel, should.BeTrue)

		// Assert task
		task := &tpb.CancelAnalysisTask{
			AnalysisId: cfa.Id,
		}
		expected := proto.Clone(task).(*tpb.CancelAnalysisTask)
		assert.Loosely(t, scheduler.Tasks().Payloads()[0], should.Match(expected))
	})
}

func TestShouldCancelAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("Should cancel analysis", t, func(t *ftt.Test) {
		fb := &model.LuciFailedBuild{
			LuciBuild: model.LuciBuild{
				GitilesCommit: buildbucketpb.GitilesCommit{
					Position: 10,
				},
				BuildNumber: 100,
			},
		}

		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		cf := testutil.CreateCompileFailure(c, t, fb)
		cfa := testutil.CreateCompileFailureAnalysis(c, t, 789, cf)

		build := &buildbucketpb.Build{
			Output: &buildbucketpb.Build_Output{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Position: 11,
				},
			},
		}

		shouldCancel, err := shouldCancelAnalysis(c, cfa, build)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, shouldCancel, should.BeTrue)

		build = &buildbucketpb.Build{
			Number: 101,
		}
		shouldCancel, err = shouldCancelAnalysis(c, cfa, build)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, shouldCancel, should.BeTrue)

		build = &buildbucketpb.Build{
			Output: &buildbucketpb.Build_Output{
				GitilesCommit: &buildbucketpb.GitilesCommit{
					Position: 9,
				},
			},
			Number: 101,
		}
		shouldCancel, err = shouldCancelAnalysis(c, cfa, build)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, shouldCancel, should.BeFalse)
	})
}
