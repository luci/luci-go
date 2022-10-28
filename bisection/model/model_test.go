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
	"testing"

	gofinditpb "go.chromium.org/luci/bisection/proto"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestDatastoreModel(t *testing.T) {
	t.Parallel()

	Convey("Datastore Model", t, func() {
		c := gaetesting.TestingContext()
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
				BuildFailureType: gofinditpb.BuildFailureType_COMPILE,
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
				Status:             gofinditpb.AnalysisStatus_FOUND,
				FirstFailedBuildId: 88000998778,
				LastPassedBuildId:  873929392903,
				InitialRegressionRange: &gofinditpb.RegressionRange{
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
				Status:         gofinditpb.AnalysisStatus_CREATED,
			}
			So(datastore.Put(c, heuristic_analysis), ShouldBeNil)

			nthsection_analysis := &CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				StartTime:      cl.Now(),
				EndTime:        cl.Now(),
				Status:         gofinditpb.AnalysisStatus_CREATED,
				BlameList: &gofinditpb.BlameList{
					Commits: []*gofinditpb.BlameListSingleCommit{
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

			culprit := &Culprit{
				ParentAnalysis: datastore.KeyForObj(c, compile_failure_analysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Project:  "my project",
					Host:     "host",
					Ref:      "ref",
					Id:       "id",
					Position: 3433,
				},
			}
			So(datastore.Put(c, culprit), ShouldBeNil)

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
	})
}
