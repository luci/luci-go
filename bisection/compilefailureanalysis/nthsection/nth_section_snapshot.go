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
	"fmt"
	"math"
	"sort"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// NthSectionSnapshot contains the current snapshot of the nth-section run
// Including the blamelist, status of the reruns...
type NthSectionSnapshot struct {
	BlameList *pb.BlameList
	// Runs are sorted by index
	Runs []*NthSectionSnapshotRun
	// We want a way to detect infinite loop where there is some "consistent" infra failure
	// for a builder, and nth section keep retrying for that builder, and
	// draining the resources.
	// In such cases, keep track of the number of infra failed rerun, and if
	// there are too many, don't run any more
	NumInfraFailed int
	NumInProgress  int
}

type NthSectionSnapshotRun struct {
	Index  int // index of the run (on the blamelist)
	Commit string
	Status pb.RerunStatus       // status of the run
	Type   model.RerunBuildType // Whether this is nth-section or culprit verification run
}

func CreateSnapshot(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis) (*NthSectionSnapshot, error) {
	// Get all reruns for the current analysis
	// This should contain all reruns for nth section and culprit verification
	q := datastore.NewQuery("SingleRerun").Eq("analysis", nthSectionAnalysis.ParentAnalysis).Order("start_time")
	reruns := []*model.SingleRerun{}
	err := datastore.GetAll(c, q, &reruns)
	if err != nil {
		return nil, errors.Annotate(err, "getting all reruns").Err()
	}

	snapshot := &NthSectionSnapshot{
		BlameList:      nthSectionAnalysis.BlameList,
		Runs:           []*NthSectionSnapshotRun{},
		NumInfraFailed: 0,
	}

	statusMap := map[string]pb.RerunStatus{}
	typeMap := map[string]model.RerunBuildType{}
	for _, r := range reruns {
		statusMap[r.GetId()] = r.Status
		typeMap[r.GetId()] = r.Type
		if r.Status == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED {
			snapshot.NumInfraFailed++
		}
		if r.Status == pb.RerunStatus_RERUN_STATUS_IN_PROGRESS {
			snapshot.NumInProgress++
		}
	}

	blamelist := nthSectionAnalysis.BlameList
	for index, cl := range blamelist.Commits {
		if stat, ok := statusMap[cl.Commit]; ok {
			snapshot.Runs = append(snapshot.Runs, &NthSectionSnapshotRun{
				Index:  index,
				Commit: cl.Commit,
				Status: stat,
				Type:   typeMap[cl.Commit],
			})
		}
	}
	return snapshot, nil
}

func (snapshot *NthSectionSnapshot) HasTooManyInfraFailure() bool {
	// TODO (nqmtuan): Move the "2" into config.
	return snapshot.NumInfraFailed > 2
}

// GetCurrentRegressionRange will return a pair of indices from the Snapshot
// that contains the culprit, based on the results of the rerun.
// Note: In the snapshot blamelist, index 0 refer to first failed,
// and index (n-1) refer to the commit after last pass
// This function will return an error if the regression range is invalid
// (there was a passed rerun which is more recent that a failed rerun)
func (snapshot *NthSectionSnapshot) GetCurrentRegressionRange() (int, int, error) {
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
	if firstFailedIdx > lastPassedIdx {
		return 0, 0, fmt.Errorf("invalid regression range - firstFailedIdx > lastPassedIdx: (%d, %d)", firstFailedIdx, lastPassedIdx)
	}
	return firstFailedIdx, lastPassedIdx - 1, nil
}

// GetCulprit returns the result of NthSection
// The first return value will be true iff there is a result
// Second value will be the index of the culprit in the blamelist
func (snapshot *NthSectionSnapshot) GetCulprit() (bool, int) {
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
func (snapshot *NthSectionSnapshot) FindNextIndicesToRun(n int) ([]int, error) {
	hasCulprit, _ := snapshot.GetCulprit()
	// There is a culprit, no need to run anymore
	if hasCulprit {
		return []int{}, nil
	}

	// Too many infra failure, we don't want to continue
	if snapshot.HasTooManyInfraFailure() {
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
// but it returns the commit hashes instead of indice
func (snapshot *NthSectionSnapshot) FindNextCommitsToRun(n int) ([]string, error) {
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
func (snapshot *NthSectionSnapshot) findRegressionChunks() ([]*NthSectionSnapshotChunk, error) {
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
