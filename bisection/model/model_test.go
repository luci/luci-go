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

	pb "go.chromium.org/luci/bisection/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestDatastoreModel(t *testing.T) {
	t.Parallel()

	Convey("Datastore Model", t, func() {
		c := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		Convey("Can create datastore models", func() {
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
			So(datastore.Put(c, failed_build), ShouldBeNil)

			compile_failure := &CompileFailure{
				Build:         datastore.KeyForObj(c, failed_build),
				OutputTargets: []string{"abc.xyx"},
				Rule:          "CXX",
				Dependencies:  []string{"dep"},
			}
			So(datastore.Put(c, compile_failure), ShouldBeNil)

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
			So(datastore.Put(c, compile_failure_analysis), ShouldBeNil)

			heuristic_analysis := &CompileHeuristicAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				StartTime:      cl.Now(),
				EndTime:        cl.Now(),
				Status:         pb.AnalysisStatus_CREATED,
			}
			So(datastore.Put(c, heuristic_analysis), ShouldBeNil)

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
			So(datastore.Put(c, nthsection_analysis), ShouldBeNil)

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
			So(datastore.Put(c, suspect), ShouldBeNil)

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
			So(datastore.Put(c, rerun_build), ShouldBeNil)
		})

		Convey("Can create TestSingleRerun models", func() {
			tf1 := &TestFailure{
				ID: 100,
			}
			tf2 := &TestFailure{
				ID: 101,
			}
			tfa := &TestFailureAnalysis{
				ID: 1000,
			}
			So(datastore.Put(c, tf1), ShouldBeNil)
			So(datastore.Put(c, tf2), ShouldBeNil)
			So(datastore.Put(c, tfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			singleRerun := &TestSingleRerun{
				AnalysisKey: datastore.KeyForObj(c, tfa),
				LuciBuild: LuciBuild{
					BuildId: 12345,
					GitilesCommit: buildbucketpb.GitilesCommit{
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
			So(datastore.Put(c, singleRerun), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			q := datastore.NewQuery("TestSingleRerun").Eq("analysis_key", datastore.KeyForObj(c, tfa))
			reruns := []*TestSingleRerun{}
			So(datastore.GetAll(c, q, &reruns), ShouldBeNil)
			So(len(reruns), ShouldEqual, 1)
			So(reruns[0], ShouldResembleProto, singleRerun)
		})
	})
}

func TestTestFailureBundle(t *testing.T) {
	t.Parallel()

	Convey("Test failure bundle", t, func() {
		bundle := &TestFailureBundle{}

		tf1 := &TestFailure{
			ID: 100,
		}
		tf2 := &TestFailure{
			ID:        101,
			IsPrimary: true,
		}
		err := bundle.Add([]*TestFailure{
			tf1,
			tf2,
		})
		So(err, ShouldBeNil)
		So(bundle.Primary(), ShouldResemble, tf2)
		So(len(bundle.Others()), ShouldEqual, 1)
		So(bundle.Others()[0], ShouldResemble, tf1)
		So(len(bundle.All()), ShouldEqual, 2)

		tf3 := &TestFailure{
			ID:        102,
			IsPrimary: true,
		}
		err = bundle.Add([]*TestFailure{
			tf3,
		})
		So(err, ShouldNotBeNil)
	})
}
