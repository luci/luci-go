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

// Package nthsectionsnapshot contains the logic for getting the current state
// for nthsection analysis and get the next commits to run.
package nthsectionsnapshot

import (
	"fmt"
	"math"
	"sort"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
)

// Snapshot contains the current snapshot of the nth-section run
// Including the blamelist, status of the reruns...
type Snapshot struct {
	BlameList *pb.BlameList
	// Runs are sorted by index
	Runs []*Run
	// We want a way to detect infinite loop where there is some "consistent" infra failure
	// for a builder, and nth section keep retrying for that builder, and
	// draining the resources.
	// In such cases, keep track of the number of infra failed rerun, and if
	// there are too many, don't run any more
	NumInfraFailed int
	// NumInProgress is the number of reruns that are currently running.
	NumInProgress int
	// NumTestSkipped is the number of reruns with the TEST_SKIPPED status.
	// It indicates that the primary test failure was not executed, so we
	// may not know the next commit for bisection.
	NumTestSkipped int
}

type Run struct {
	// Index of the run (on the blamelist).
	Index  int
	Commit string
	Status pb.RerunStatus       // status of the run
	Type   model.RerunBuildType // Whether this is nth-section or culprit verification run
}

func (snapshot *Snapshot) HasTooManyInfraFailure() bool {
	return snapshot.NumInfraFailed > 1
}

// BadRangeError suggests the regression range is invalid.
// For example, if a passed rerun is found that is more recent
// than a failed rerun, the regression range is invalid.
type BadRangeError struct {
	FirstFailedIdx int
	LastPassedIdx  int
}

func (b *BadRangeError) Error() string {
	return fmt.Sprintf("invalid regression range - firstFailedIdx >= lastPassedIdx: (%d, %d)", b.FirstFailedIdx, b.LastPassedIdx)
}

// GetCurrentRegressionRange will return a pair of indices from the Snapshot
// that contains the culprit, based on the results of the rerun.
// Note: In the snapshot blamelist, index 0 refer to first failed,
// and index (n-1) refer to the commit after last pass.
// This function will return an BadRangeError if the regression range is invalid.
func (snapshot *Snapshot) GetCurrentRegressionRange() (int, int, error) {
	firstFailedIdx := 0
	lastPassedIdx := len(snapshot.BlameList.Commits)
	for _, run := range snapshot.Runs {
		// The snapshot runs are sorted by index, so we don't need the (firstFailedIdx < run.Index) check here
		if run.Status == pb.RerunStatus_RERUN_STATUS_FAILED {
			firstFailedIdx = run.Index
		}
		if run.Status == pb.RerunStatus_RERUN_STATUS_PASSED {
			if run.Index < lastPassedIdx {
				lastPassedIdx = run.Index
			}
		}
	}
	if firstFailedIdx >= lastPassedIdx {
		return 0, 0, &BadRangeError{
			FirstFailedIdx: firstFailedIdx,
			LastPassedIdx:  lastPassedIdx,
		}
	}
	return firstFailedIdx, lastPassedIdx - 1, nil
}

// GetCulprit returns the result of NthSection
// The first return value will be true iff there is a result
// Second value will be the index of the culprit in the blamelist
func (snapshot *Snapshot) GetCulprit() (bool, int) {
	// GetCurrentRegressionRange returns the range that contain the culprit
	start, end, err := snapshot.GetCurrentRegressionRange()
	// If err != nil, it means last pass is later than first failed
	// In such case, no culprit is found.
	if err != nil {
		return false, 0
	}
	// We haven't found the culprit yet
	if start != end {
		return false, 0
	}
	// The regression range only has 1 element: it is the culprit
	return true, start
}

type NthSectionSnapshotChunk struct {
	Begin int
	End   int
}

func (chunk *NthSectionSnapshotChunk) length() int {
	return chunk.End - chunk.Begin + 1
}

// FindNextIndicesToRun finds at most n next commits to run for nthsection.
// At most n indices will be returned
// The target is to minimize the biggest chunks
// For example, if the regression is [0..9], and n=3,
// We can run at indices 2, 5, 8 to break the range into 4 "chunks"
// [0-1], [3-4], [6-7], [9]. The biggest chunk is of size 2.
func (snapshot *Snapshot) FindNextIndicesToRun(n int) ([]int, error) {
	hasCulprit, _ := snapshot.GetCulprit()
	// There is a culprit, no need to run anymore
	if hasCulprit {
		return []int{}, nil
	}

	// Too many infra failure, we don't want to continue
	if snapshot.HasTooManyInfraFailure() {
		return []int{}, nil
	}

	if snapshot.NumTestSkipped > 0 {
		return []int{}, nil
	}

	chunks, err := snapshot.findRegressionChunks()
	if err != nil {
		return nil, err
	}

	// Use n "dividers" to divide those chunks into even smaller chunks
	// such that the max of those smaller chunks is minimized.
	// We are not optimizing for speed here, because in reality, the number of chunks
	// and n will be very small.
	// We are using a brute force (recursive) method here.
	allocations, _ := chunking(chunks, 0, n, n)
	result := []int{}
	for i, chunk := range chunks {
		result = append(result, breakToSmallerChunks(chunk, allocations[i])...)
	}
	return result, nil
}

// FindNextCommitsToRun is similar to FindNextIndicesToRun,
// but it returns the commit hashes instead of indices.
func (snapshot *Snapshot) FindNextCommitsToRun(n int) ([]string, error) {
	indices, err := snapshot.FindNextIndicesToRun(n)
	if err != nil {
		return nil, err
	}
	commits := make([]string, len(indices))
	for i, index := range indices {
		commits[i] = snapshot.BlameList.Commits[index].Commit
	}
	return commits, nil
}

// findRegressionChunks finds the regression range and breaks it into chunks
// the result will be sorted (biggest chunk will come first)
func (snapshot *Snapshot) findRegressionChunks() ([]*NthSectionSnapshotChunk, error) {
	start, end, err := snapshot.GetCurrentRegressionRange()
	if err != nil {
		return nil, err
	}

	// Find the indices of running builds in the regression range
	// We should not run again for those builds, but instead, we should
	// use those builds to break the range into smaller chunks
	chunks := []*NthSectionSnapshotChunk{}
	for _, run := range snapshot.Runs {
		// There is a special case where there is a failed run at the start
		// In such case we don't want to include the failed run in any chunks
		if run.Index == start && run.Status == pb.RerunStatus_RERUN_STATUS_FAILED {
			start = run.Index + 1
			continue
		}
		if run.Index >= start && run.Index <= end && run.Status == pb.RerunStatus_RERUN_STATUS_IN_PROGRESS {
			if start <= run.Index-1 {
				chunks = append(chunks, &NthSectionSnapshotChunk{Begin: start, End: run.Index - 1})
			}
			start = run.Index + 1
		}
	}
	if start <= end {
		chunks = append(chunks, &NthSectionSnapshotChunk{Begin: start, End: end})
	}

	// Sort the chunks descendingly based on length
	// In general, the "bigger" chunks should be allocated more "dividers"
	sort.Slice(chunks, func(i, j int) bool {
		return chunks[i].length() > chunks[j].length()
	})
	return chunks, nil
}

// Use n dividers to divide chunks
// The chunks are sorted by length descendingly
// We only consider chunks from the start index
// Return the array of allocation and the biggest chunk size
// maxAllocationForEachChunk is to control the allocation: there is not cases
// where we want to allocate more dividers to a smaller chunks
func chunking(chunks []*NthSectionSnapshotChunk, start int, nDivider int, maxAllocationForEachChunk int) ([]int, int) {
	// Base case: There is no chunk
	// It may mean that all applicable commits for rerun are in progress
	if len(chunks) == 0 {
		return []int{0}, 0
	}
	// Base case: Only one chunk left
	if start == len(chunks)-1 {
		return []int{nDivider}, calculateChunkSize(chunks[start].length(), nDivider)
	}
	// Base case: No Divider left -> return the biggest chunk
	if nDivider == 0 {
		return zerosSlice(len(chunks) - start + 1), chunks[start].length()
	}
	// Recursive, k is the number of dividers allocated the "start" chunk
	dividerLeft := minInt(nDivider, maxAllocationForEachChunk)
	min := math.MaxInt64
	allocation := []int{}
	for k := dividerLeft; k > 0; k-- {
		startSize := calculateChunkSize(chunks[start].length(), k)
		// We passed k here because we don't want to allocate more dividers to a smaller chunk
		// The recursion depth here is limited by the chunks length and nDivider, which in reality
		// should be < 10
		subAllocation, otherSize := chunking(chunks, start+1, nDivider-k, k)
		if min > maxInt(startSize, otherSize) {
			min = maxInt(startSize, otherSize)
			allocation = append([]int{k}, subAllocation...)
		}
	}
	return allocation, min
}

// Create a zeroes slice with length l
func zerosSlice(l int) []int {
	s := make([]int, l)
	for i := range s {
		s[i] = 0
	}
	return s
}

func minInt(a int, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

func maxInt(a int, b int) int {
	return int(math.Max(float64(a), float64(b)))
}

// With the initial length, if we use n dividers to divide as equally as possible
// then how long each chunk will be?
// Example: initialLength = 10, nDivider = 3 -> chunk size = 2
// Example: initialLength = 3, nDivider = 1 -> chunk size = 1
func calculateChunkSize(initialLength int, nDivider int) int {
	if nDivider >= initialLength {
		return 0
	}
	return int(math.Ceil(float64(initialLength-nDivider) / (float64(nDivider + 1))))
}

// return the indices for the break points
func breakToSmallerChunks(chunk *NthSectionSnapshotChunk, nDivider int) []int {
	if nDivider > chunk.length() {
		nDivider = chunk.length()
	}
	step := float64(chunk.length()-nDivider) / float64(nDivider+1)
	result := []int{}
	for i := 1; i <= nDivider; i++ {
		next := int(math.Round(step*float64(i) + float64(i-1)))
		// next >= chunk.length() should not happen, but just in case
		if next < chunk.length() {
			result = append(result, chunk.Begin+next)
		}
	}

	return result
}

// FindNextSingleCommitToRun returns the next commit to run.
// Used to get the new rerun when we get the update from recipe.
// If we cannot find the next commit, we will return empty string.
func (snapshot *Snapshot) FindNextSingleCommitToRun() (string, error) {
	// We pass 1 as argument here because we only need to find one commit
	// to replace the finishing one.
	commits, err := snapshot.FindNextCommitsToRun(1)
	if err != nil {
		return "", errors.Annotate(err, "find next commits to run").Err()
	}
	// There is no commit to run, perhaps we already found a culprit, or we
	// have already scheduled the necessary build to be run.
	if len(commits) == 0 {
		return "", nil
	}
	if len(commits) != 1 {
		return "", errors.Reason("expect only 1 commits to rerun. Got %d", len(commits)).Err()
	}
	return commits[0], nil

}
