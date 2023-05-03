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

package bayesian

// changePointHeuristic identifies points in a test variant history
// which have the potential of being the most likely position for a
// change point.
//
// This is used to avoid expensive statistical calculations evaluating
// non-credible change point positions.
//
// This heuristic is only valid for the bayesian likelihood model
// used in this file.
//
// This heuristic is consistent with the approach in the sense that
// it will never eliminate positions which are the most likely position,
// assuming that:
//   - the most likely position is also more likely than the null hypothesis,
//   - each change point position (other than the 'no change point') is
//     equally likely.
//
// It may however identify positions that (upon detailed statistical
// evaluation) are not a likely change point location.
//
// The heuristic works by identifying points in the history where
// there is an expected-to-unexpected transition (or vice-versa).
//
// Usage (where positionCounts contains statistics for each commit
// position, in order of commit position):
//
//	var heuristic changePointHeuristic
//	heuristic.AddToHistory(positionCounts[0])
//
//	for i := 1; i < len(positionCounts); i++ {
//		if heuristic.isPossibleChangepoint(positionCounts[i]) {
//			// Consider a change point between i-1 and i in more detail.
//		}
//		heuristic.addToHistory(positionCounts[i])
//	}
type changePointHeuristic struct {
	addedExpectedRuns         bool
	addedUnexpectedRuns       bool
	addedExpectedAfterRetry   bool
	addedUnexpectedAfterRetry bool
}

// addToHistory updates the heuristic state to reflect the given
// additional history. History should be input one commit position
// at a time.
func (h *changePointHeuristic) addToHistory(positionCounts counts) {
	if positionCounts.Runs > 0 {
		h.addedUnexpectedRuns = positionCounts.HasUnexpected > 0
		h.addedExpectedRuns = (positionCounts.Runs - positionCounts.HasUnexpected) > 0
	}
	if positionCounts.Retried > 0 {
		h.addedUnexpectedAfterRetry = positionCounts.UnexpectedAfterRetry > 0
		h.addedExpectedAfterRetry = (positionCounts.Retried - positionCounts.UnexpectedAfterRetry) > 0
	}
}

// isChangepointPossibleWithNext returns whether there could be a
// change point between the history consumed so far on the left,
// and the remainder of the history on the right.
// nextPositionCounts is the counts of the next commit position
// on the right.
func (h changePointHeuristic) isChangepointPossibleWithNext(nextPositionCounts counts) bool {
	consider := false
	if nextPositionCounts.Runs > 0 {
		// Detect expected-to-unexpected transition (or vica-versa).
		addingUnexpectedRuns := nextPositionCounts.HasUnexpected > 0
		addingExpectedRuns := (nextPositionCounts.Runs - nextPositionCounts.HasUnexpected) > 0
		consider = consider || addingUnexpectedRuns && h.addedExpectedRuns
		consider = consider || addingExpectedRuns && h.addedUnexpectedRuns
	}
	if nextPositionCounts.Retried > 0 {
		// Detect expected-to-unexpected transition in the result
		// after retries (or vica-versa).
		addingUnexpectedAfterRetry := nextPositionCounts.UnexpectedAfterRetry > 0
		addingExpectedAfterRetry := (nextPositionCounts.Retried - nextPositionCounts.UnexpectedAfterRetry) > 0
		consider = consider || addingUnexpectedAfterRetry && h.addedExpectedAfterRetry
		consider = consider || addingExpectedAfterRetry && h.addedUnexpectedAfterRetry
	}
	return consider
}
