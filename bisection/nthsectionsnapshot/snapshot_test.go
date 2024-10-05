// Copyright 2023 The LUCI Authors.
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

package nthsectionsnapshot

import (
	"testing"

	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChunking(t *testing.T) {
	t.Parallel()

	ftt.Run("One chunk", t, func(t *ftt.Test) {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 3, 10)
		assert.Loosely(t, alloc, should.Resemble([]int{3}))
		assert.Loosely(t, chunkSize, should.Equal(2))
	})

	ftt.Run("One chunk, no divider", t, func(t *ftt.Test) {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 0, 10)
		assert.Loosely(t, alloc, should.Resemble([]int{0}))
		assert.Loosely(t, chunkSize, should.Equal(10))
	})

	ftt.Run("One chunk, one divider", t, func(t *ftt.Test) {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 1, 1)
		assert.Loosely(t, alloc, should.Resemble([]int{1}))
		assert.Loosely(t, chunkSize, should.Equal(5))
	})

	ftt.Run("One chunk, many dividers", t, func(t *ftt.Test) {
		chunks := []*NthSectionSnapshotChunk{
			{
				Begin: 0,
				End:   9,
			},
		}
		alloc, chunkSize := chunking(chunks, 0, 100, 100)
		assert.Loosely(t, alloc, should.Resemble([]int{100}))
		assert.Loosely(t, chunkSize, should.BeZero)
	})

	ftt.Run("Two equal chunks", t, func(t *ftt.Test) {
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
		assert.Loosely(t, alloc, should.Resemble([]int{1, 0}))
		assert.Loosely(t, chunkSize, should.Equal(10))
	})

	ftt.Run("Two equal chunks, two dividers", t, func(t *ftt.Test) {
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
		assert.Loosely(t, alloc, should.Resemble([]int{1, 1}))
		assert.Loosely(t, chunkSize, should.Equal(5))
	})

	ftt.Run("Two unequal chunks, two dividers", t, func(t *ftt.Test) {
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
		assert.Loosely(t, alloc, should.Resemble([]int{2, 0}))
		assert.Loosely(t, chunkSize, should.Equal(3))
	})

	ftt.Run("Two unequal chunks, many dividers", t, func(t *ftt.Test) {
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
		assert.Loosely(t, alloc, should.Resemble([]int{5, 1}))
		assert.Loosely(t, chunkSize, should.Equal(1))
	})

	ftt.Run("Three chunks", t, func(t *ftt.Test) {
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
		assert.Loosely(t, alloc, should.Resemble([]int{2, 1, 1}))
		assert.Loosely(t, chunkSize, should.Equal(3))
	})
}

func TestGetRegressionRange(t *testing.T) {
	t.Parallel()

	ftt.Run("GetRegressionRangeNoRun", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs:      []*Run{},
		}
		ff, lp, err := snapshot.GetCurrentRegressionRange()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ff, should.BeZero)
		assert.Loosely(t, lp, should.Equal(99))
	})

	ftt.Run("GetRegressionRangeOK", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ff, should.Equal(15))
		assert.Loosely(t, lp, should.Equal(39))
	})

	ftt.Run("GetRegressionRangeError", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("GetRegressionRangeErrorFirstFailedEqualsLastPass", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
				{
					Index:  0,
					Status: pb.RerunStatus_RERUN_STATUS_PASSED,
				},
			},
		}
		_, _, err := snapshot.GetCurrentRegressionRange()
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestGetCulprit(t *testing.T) {
	t.Parallel()

	ftt.Run("GetCulpritOK", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		ok, cul := snapshot.GetCulprit()
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, cul, should.Equal(15))
	})

	ftt.Run("GetCulpritFailed", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs:      []*Run{},
		}
		ok, _ := snapshot.GetCulprit()
		assert.Loosely(t, ok, should.BeFalse)
	})

	ftt.Run("GetCulpritError", t, func(t *ftt.Test) {
		// Create a blamelist with 100 commit
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		ok, _ := snapshot.GetCulprit()
		assert.Loosely(t, ok, should.BeFalse)
	})

}

func TestFindRegressionChunks(t *testing.T) {
	t.Parallel()

	ftt.Run("findRegressionChunks", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(100)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, chunks, should.Resemble([]*NthSectionSnapshotChunk{
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
		}))
	})
}

func TestBreakIntoSmallerChunks(t *testing.T) {
	t.Parallel()

	ftt.Run("break", t, func(t *ftt.Test) {
		chunk := &NthSectionSnapshotChunk{
			Begin: 10,
			End:   19,
		}
		assert.Loosely(t, breakToSmallerChunks(chunk, 0), should.Resemble([]int{}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 1), should.Resemble([]int{15}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 2), should.Resemble([]int{13, 16}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 3), should.Resemble([]int{12, 15, 17}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 4), should.Resemble([]int{11, 13, 16, 18}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 5), should.Resemble([]int{11, 13, 15, 16, 18}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 6), should.Resemble([]int{11, 12, 14, 15, 17, 18}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 10), should.Resemble([]int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}))
		assert.Loosely(t, breakToSmallerChunks(chunk, 100), should.Resemble([]int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}))
	})
}

func TestFindNextIndicesToRun(t *testing.T) {
	t.Parallel()

	ftt.Run("FindNextIndicesToRun", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
				{
					Index:  5,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
			},
		}
		indices, err := snapshot.FindNextIndicesToRun(2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, indices, should.Resemble([]int{2, 8}))
	})

	ftt.Run("FindNextIndicesToRun with a single commit should not return anything", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(1)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs:      []*Run{},
		}
		indices, err := snapshot.FindNextIndicesToRun(2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, indices, should.Resemble([]int{}))
	})

	ftt.Run("FindNextIndicesToRun already found culprit", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, indices, should.Resemble([]int{}))
	})

	ftt.Run("FindNextIndicesToRunAllRerunsAreRunning", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(3)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
				{
					Index:  0,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  1,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
				{
					Index:  2,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
			},
		}
		indices, err := snapshot.FindNextIndicesToRun(2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, indices, should.Resemble([]int{}))
	})
}

func TestFindNextCommitsToRun(t *testing.T) {
	t.Parallel()

	ftt.Run("FindNextIndicesToRun", t, func(t *ftt.Test) {
		blamelist := testutil.CreateBlamelist(10)
		snapshot := &Snapshot{
			BlameList: blamelist,
			Runs: []*Run{
				{
					Index:  5,
					Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				},
			},
		}
		indices, err := snapshot.FindNextCommitsToRun(2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, indices, should.Resemble([]string{"commit2", "commit8"}))
	})
}

func TestCalculateChunkSize(t *testing.T) {
	t.Parallel()

	ftt.Run("CalculateChunkSize", t, func(t *ftt.Test) {
		assert.Loosely(t, calculateChunkSize(5, 10), should.BeZero)
		assert.Loosely(t, calculateChunkSize(5, 5), should.BeZero)
		assert.Loosely(t, calculateChunkSize(10, 3), should.Equal(2))
		assert.Loosely(t, calculateChunkSize(10, 0), should.Equal(10))
		assert.Loosely(t, calculateChunkSize(3, 1), should.Equal(1))
	})
}
