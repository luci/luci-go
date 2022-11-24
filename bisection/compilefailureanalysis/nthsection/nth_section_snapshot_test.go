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

package nthsection

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/smartystreets/goconvey/convey"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestChunking(t *testing.T) {
	t.Parallel()

	Convey("One chunk", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 3, 10)
		So(alloc, ShouldResemble, []int{3})
		So(chunkSize, ShouldEqual, 2)
	})

	Convey("One chunk, no divider", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 0, 10)
		So(alloc, ShouldResemble, []int{0})
		So(chunkSize, ShouldEqual, 10)
	})

	Convey("One chunk, one divider", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 1, 1)
		So(alloc, ShouldResemble, []int{1})
		So(chunkSize, ShouldEqual, 5)
	})

	Convey("One chunk, many dividers", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 100, 100)
		So(alloc, ShouldResemble, []int{100})
		So(chunkSize, ShouldEqual, 0)
	})

	Convey("Two equal chunks", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
			{
				Begin: 10,
				End:   19,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 1, 1)
		So(alloc, ShouldResemble, []int{1, 0})
		So(chunkSize, ShouldEqual, 10)
	})

	Convey("Two equal chunks, two dividers", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
			{
				Begin: 10,
				End:   19,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 2, 2)
		So(alloc, ShouldResemble, []int{1, 1})
		So(chunkSize, ShouldEqual, 5)
	})

	Convey("Two unequal chunks, two dividers", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
			{
				Begin: 10,
				End:   11,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 2, 2)
		So(alloc, ShouldResemble, []int{2, 0})
		So(chunkSize, ShouldEqual, 3)
	})

	Convey("Two unequal chunks, many dividers", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
			{
				Begin: 10,
				End:   11,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 6, 6)
		So(alloc, ShouldResemble, []int{5, 1})
		So(chunkSize, ShouldEqual, 1)
	})

	Convey("Three chunks", t, func() {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
			{
				Begin: 10,
				End:   15,
			},
			{
				Begin: 16,
				End:   19,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 4, 4)
		So(alloc, ShouldResemble, []int{2, 1, 1})
		So(chunkSize, ShouldEqual, 3)
	})
}

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
		Kind: "SingleRerun",
		SortBy: []datastore.IndexColumn{
			{
				Property: "analysis",
			},
			{
				Property: "start_time",
			},
		},
	})

	Convey("Create Snapshot", t, func() {
		analysis := &model.CompileFailureAnalysis{}
		So(datastore.Put(c, analysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		blamelist := testutil.CreateBlamelist(4)
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{
			BlameList:      blamelist,
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, nthSectionAnalysis), ShouldBeNil)

		rerun1 := &model.SingleRerun{
			Type:   model.RerunBuildType_CulpritVerification,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Id: "commit1",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun1), ShouldBeNil)

		rerun2 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Id: "commit3",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, rerun2), ShouldBeNil)

		rerun3 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Id: "commit0",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun3), ShouldBeNil)

		rerun4 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			GitilesCommit: buildbucketpb.GitilesCommit{
				Id: "commit2",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun4), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		snapshot, err := CreateSnapshot(c, nthSectionAnalysis)
		So(err, ShouldBeNil)
		diff := cmp.Diff(snapshot.BlameList, blamelist, cmp.Comparer(proto.Equal))
		So(diff, ShouldEqual, "")

		So(snapshot.NumInProgress, ShouldEqual, 2)
		So(snapshot.NumInfraFailed, ShouldEqual, 1)
		So(snapshot.Runs, ShouldResemble, []*NthSectionSnapshotRun{
			{
				Index:  0,
				Commit: "commit0",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  1,
				Commit: "commit1",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_CulpritVerification,
			},
			{
				Index:  2,
				Commit: "commit2",
				Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  3,
				Commit: "commit3",
				Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
		})
	})
}

func TestGetRegressionRange(t *testing.T) {
	t.Parallel()

	Convey("GetRegressionRangeNoRun", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs:      []*NthSectionSnapshotRun{},
		}
		ff, lp, err := snapshot.GetCurrentRegressionRange()
		So(err, ShouldBeNil)
		So(ff, ShouldEqual, 0)
		So(lp, ShouldEqual, 99)
	})

	Convey("GetRegressionRangeOK", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  10,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  15,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  40,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
				{
					Index:  50,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		ff, lp, err := snapshot.GetCurrentRegressionRange()
		So(err, ShouldBeNil)
		So(ff, ShouldEqual, 15)
		So(lp, ShouldEqual, 39)
	})

	Convey("GetRegressionRangeError", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  17,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  10,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		_, _, err := snapshot.GetCurrentRegressionRange()
		So(err, ShouldNotBeNil)
	})
}

func TestGetCulprit(t *testing.T) {
	t.Parallel()

	Convey("GetCulpritOK", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  15,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  16,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		ok, cul, err := snapshot.GetCulprit()
		So(ok, ShouldBeTrue)
		So(cul, ShouldEqual, 15)
		So(err, ShouldBeNil)
	})

	Convey("GetCulpritFailed", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs:      []*NthSectionSnapshotRun{},
		}
		ok, _, err := snapshot.GetCulprit()
		So(ok, ShouldBeFalse)
		So(err, ShouldBeNil)
	})

	Convey("GetCulpritError", t, func() {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  10,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  2,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		_, _, err := snapshot.GetCulprit()
		So(err, ShouldNotBeNil)
	})

}

func TestFindRegressionChunks(t *testing.T) {
	t.Parallel()

	Convey("findRegressionChunks", t, func() {
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  15,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				{
					Index:  19,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  26,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  35,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  39,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  40,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		chunks, err := snapshot.findRegressionChunks()
		So(err, ShouldBeNil)
		So(chunks, ShouldResemble, []*NthSectionSnapshotChunk{
			{
				Begin: 27,
				End:   34,
			},
			{
				Begin: 20,
				End:   25,
			},
			{
				Begin: 16,
				End:   18,
			},
			{
				Begin: 36,
				End:   38,
			},
		})
	})
}

func TestBreakIntoSmallerChunks(t *testing.T) {
	t.Parallel()

	Convey("break", t, func() {
		chunk := &NthSectionSnapshotChunk{
			Begin: 10,
			End:   19,
		}
		So(breakToSmallerChunks(chunk, 0), ShouldResemble, []int{})
		So(breakToSmallerChunks(chunk, 1), ShouldResemble, []int{15})
		So(breakToSmallerChunks(chunk, 2), ShouldResemble, []int{13, 16})
		So(breakToSmallerChunks(chunk, 3), ShouldResemble, []int{12, 15, 17})
		So(breakToSmallerChunks(chunk, 4), ShouldResemble, []int{11, 13, 16, 18})
		So(breakToSmallerChunks(chunk, 5), ShouldResemble, []int{11, 13, 15, 16, 18})
		So(breakToSmallerChunks(chunk, 6), ShouldResemble, []int{11, 12, 14, 15, 17, 18})
		So(breakToSmallerChunks(chunk, 10), ShouldResemble, []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
		So(breakToSmallerChunks(chunk, 100), ShouldResemble, []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19})
	})
}

func TestFindNextIndicesToRun(t *testing.T) {
	t.Parallel()

	Convey("FindNextIndicesToRun", t, func() {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  5,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
			},
		}
		indices, err := snapshot.FindNextIndicesToRun(2)
		So(err, ShouldBeNil)
		So(indices, ShouldResemble, []int{2, 8})
	})

	Convey("FindNextIndicesToRun already found culprit", t, func() {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  5,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
				{
					Index:  4,
					Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
			},
		}
		indices, err := snapshot.FindNextIndicesToRun(2)
		So(err, ShouldBeNil)
		So(indices, ShouldResemble, []int{})
	})

}

func TestFindNextCommitsToRun(t *testing.T) {
	t.Parallel()

	Convey("FindNextIndicesToRun", t, func() {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &NthSectionSnapshot{
			BlameList: blamelist,
			Runs: []*NthSectionSnapshotRun{
				{
					Index:  5,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
			},
		}
		indices, err := snapshot.FindNextCommitsToRun(2)
		So(err, ShouldBeNil)
		So(indices, ShouldResemble, []string{"commit2", "commit8"})
	})
}

func TestCalculateChunkSize(t *testing.T) {
	t.Parallel()

	Convey("CalculateChunkSize", t, func() {
		So(calculateChunkSize(5, 10), ShouldEqual, 0)
		So(calculateChunkSize(5, 5), ShouldEqual, 0)
		So(calculateChunkSize(10, 3), ShouldEqual, 2)
		So(calculateChunkSize(10, 0), ShouldEqual, 10)
		So(calculateChunkSize(3, 1), ShouldEqual, 1)
	})
}
