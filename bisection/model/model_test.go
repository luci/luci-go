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

package model

import (
	"context"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/bisection/proto/v1"
)

func TestDatastoreModel(t *testing.T) {
	t.Parallel()

	ftt.Run("Datastore Model", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		t.Run("Can create datastore models", func(t *ftt.Test) {
			failed_build := &LuciFailedBuild{
				Id: 88128398584903,
				LuciBuild: LuciBuild{
					BuildId:     88128398584903,
					Project:     "chromium",
					Bucket:      "ci",
					Builder:     "android",
					BuildNumber: 123,
					Status:      buildbucketpb.Status_FAILURE,
					StartTime:   cl.Now(),
					EndTime:     cl.Now(),
					CreateTime:  cl.Now(),
				},
				BuildFailureType: pb.BuildFailureType_COMPILE,
			}
			assert.Loosely(t, datastore.Put(c, failed_build), should.BeNil)

			compile_failure := &CompileFailure{
				Build:         datastore.KeyForObj(c, failed_build),
				OutputTargets: []string{"abc.xyx"},
				Rule:          "CXX",
				Dependencies:  []string{"dep"},
			}
			assert.Loosely(t, datastore.Put(c, compile_failure), should.BeNil)

			compile_failure_analysis := &CompileFailureAnalysis{
				CompileFailure:     datastore.KeyForObj(c, compile_failure),
				CreateTime:         cl.Now(),
				StartTime:          cl.Now(),
				EndTime:            cl.Now(),
				Status:             pb.AnalysisStatus_FOUND,
				FirstFailedBuildId: 88000998778,
				LastPassedBuildId:  873929392903,
				InitialRegressionRange: &pb.RegressionRange{
					LastPassed: &buildbucketpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
						Id:      "id1",
					},
					FirstFailed: &buildbucketpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Ref:     "ref",
						Id:      "id2",
					},
					NumberOfRevisions: 10,
				},
			}
			assert.Loosely(t, datastore.Put(c, compile_failure_analysis), should.BeNil)

			heuristic_analysis := &CompileHeuristicAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				StartTime:      cl.Now(),
				EndTime:        cl.Now(),
				Status:         pb.AnalysisStatus_CREATED,
			}
			assert.Loosely(t, datastore.Put(c, heuristic_analysis), should.BeNil)

			nthsection_analysis := &CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				StartTime:      cl.Now(),
				EndTime:        cl.Now(),
				Status:         pb.AnalysisStatus_CREATED,
				BlameList: &pb.BlameList{
					Commits: []*pb.BlameListSingleCommit{
						{
							Commit:      "12345",
							ReviewUrl:   "https://this/is/review/url/1",
							ReviewTitle: "commit 1",
						},
						{
							Commit:      "12346",
							ReviewUrl:   "https://this/is/review/url/2",
							ReviewTitle: "commit 2",
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(c, nthsection_analysis), should.BeNil)

			suspect := &Suspect{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Project:  "my project",
					Host:     "host",
					Ref:      "ref",
					Id:       "id",
					Position: 3433,
				},
				ReviewUrl:     "http://review-url.com",
				ReviewTitle:   "Added new functionality",
				Score:         100,
				Justification: "The CL touch the file abc.cc, and it is in the log",
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)

			rerun_build := &CompileRerunBuild{
				LuciBuild: LuciBuild{
					BuildId:     88128398584903,
					Project:     "chromium",
					Bucket:      "ci",
					Builder:     "android",
					BuildNumber: 123,
					Status:      buildbucketpb.Status_SUCCESS,
					StartTime:   cl.Now(),
					EndTime:     cl.Now(),
					CreateTime:  cl.Now(),
				},
			}
			assert.Loosely(t, datastore.Put(c, rerun_build), should.BeNil)
		})

		t.Run("Can create TestSingleRerun models", func(t *ftt.Test) {
			tf1 := &TestFailure{
				ID: 100,
			}
			tf2 := &TestFailure{
				ID: 101,
			}
			tfa := &TestFailureAnalysis{
				ID: 1000,
			}
			assert.Loosely(t, datastore.Put(c, tf1), should.BeNil)
			assert.Loosely(t, datastore.Put(c, tf2), should.BeNil)
			assert.Loosely(t, datastore.Put(c, tfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			singleRerun := &TestSingleRerun{
				AnalysisKey: datastore.KeyForObj(c, tfa),
				LUCIBuild: LUCIBuild{
					BuildID: 12345,
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Id:      "id",
					},
				},
				Dimensions: &pb.Dimensions{
					Dimensions: []*pb.Dimension{
						{
							Key:   "k",
							Value: "v",
						},
					},
				},
				Type: RerunBuildType_NthSection,
				TestResults: RerunTestResults{
					IsFinalized: true,
					Results: []RerunSingleTestResult{
						{
							TestFailureKey: datastore.KeyForObj(c, tf1),
							ExpectedCount:  1,
						},
						{
							TestFailureKey:  datastore.KeyForObj(c, tf2),
							UnexpectedCount: 1,
						},
					},
				},
				Status: pb.RerunStatus_RERUN_STATUS_PASSED,
			}
			assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			q := datastore.NewQuery("TestSingleRerun").Eq("analysis_key", datastore.KeyForObj(c, tfa))
			reruns := []*TestSingleRerun{}
			assert.Loosely(t, datastore.GetAll(c, q, &reruns), should.BeNil)
			assert.Loosely(t, len(reruns), should.Equal(1))
			assert.Loosely(t, reruns[0], should.Match(singleRerun))
		})
	})
}

func TestTestFailureBundle(t *testing.T) {
	t.Parallel()

	ftt.Run("Test failure bundle", t, func(t *ftt.Test) {
		bundle := &TestFailureBundle{}

		tf1 := &TestFailure{
			ID:         100,
			IsDiverged: true,
		}
		tf2 := &TestFailure{
			ID:        101,
			IsPrimary: true,
		}
		err := bundle.Add([]*TestFailure{
			tf1,
			tf2,
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bundle.Primary(), should.Match(tf2))
		assert.Loosely(t, len(bundle.Others()), should.Equal(1))
		assert.Loosely(t, bundle.Others()[0], should.Match(tf1))
		assert.Loosely(t, len(bundle.All()), should.Equal(2))

		tf3 := &TestFailure{
			ID:        102,
			IsPrimary: true,
		}
		err = bundle.Add([]*TestFailure{
			tf3,
		})
		assert.Loosely(t, err, should.NotBeNil)

		tf4 := &TestFailure{
			ID: 103,
		}
		err = bundle.Add([]*TestFailure{
			tf4,
		})
		assert.Loosely(t, err, should.BeNil)
		nonDiverged := bundle.NonDiverged()
		assert.Loosely(t, len(nonDiverged), should.Equal(2))
		assert.Loosely(t, nonDiverged[0].ID, should.Equal(101))
		assert.Loosely(t, nonDiverged[1].ID, should.Equal(103))
	})
}
