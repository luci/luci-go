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

package sequence

import (
	"go.chromium.org/luci/common/errors"
)

// In checks this pattern against the given sequence.
//
// If this Pattern is malformed, panics; If you created Pattern with NewPattern
// and it didn't return an error, the Pattern is not malformed and this method
// will not panic.
//
// By default In matches the pattern sequence anywhere; You can constrain
// this by setting Edge at the start or end of the pattern.
//
// Examples:
//
//	// true
//	NewPattern("foo").In("foo", "bar", "baz")
//	NewPattern("/a/").In("foo", "bar", "baz")
//	NewPattern("foo", "bar").In("foo", "bar", "baz")
//	NewPattern("/o$/", "bar", "/^b/").In("foo", "bar", "baz")
//	NewPattern("foo", "...", "bar").In("foo", "a", "b", "bar")
//	NewPattern("foo", "...", "bar").In("foo", "bar")
//	NewPattern("^", "bar", "baz").In("bar", "baz", "foo")
//
//	// false
//	NewPattern("^", "bar", "baz").In("foo", "bar", "baz")
func (p Pattern) In(seq ...string) bool {
	if len(p) == 0 {
		return true
	}

	// Ellipsis and Edge have a minimum width of 0, everything else has a match
	// size of 1 slot. Do a pass over matchers to calculate the number of slots
	// required to match `p[i:]`.
	minSlotsCount := 0
	minSlots := make([]int, len(p))
	prevEllipsis := false
	for i, matcher := range p {
		isEdge := matcher == Edge
		isEllipsis := matcher == Ellipsis
		if isEdge && i > 0 && i < len(p)-1 {
			panic(errors.Reason("cannot have Edge in the middle of a Pattern (i=%d)", i).Err())
		}
		if isEllipsis {
			if prevEllipsis {
				panic(errors.Reason("cannot have multiple Ellipsis in a row (i=%d)", i).Err())
			}
			prevEllipsis = true
		} else {
			prevEllipsis = false
		}
		if !isEllipsis && !isEdge {
			minSlotsCount++
		}
		minSlots[len(minSlots)-1-i] = minSlotsCount
	}
	// If p looked like ['a', 'b', ..., 'c'], minSlots now looks like:
	// [3, 2, 1, 1]

	var cachedMatchesSeq func(pOffset, seqOffset int) bool

	matchesPattern := func(pOffset, seqOffset int) bool {
		maxSlot := len(seq) - minSlots[pOffset]
		if maxSlot < seqOffset {
			return false
		}

		for seqIdx := seqOffset; seqIdx < maxSlot+1; seqIdx++ {
			numMatched := 0
			matches := true
			for matcherIdx := pOffset; matcherIdx < len(p); matcherIdx++ {
				matcher := p[matcherIdx]

				if matcher == Edge {
					// edge is a 0-width match if the current sequence item is the start
					// or end of the sequence.
					//
					// Note that we compare against len(seq) rather than len(seq)-1,
					// because a 1-length sequence would, in fact, have seqIdx==0 and
					// numMatched==1.
					if (seqIdx+numMatched) == 0 || (seqIdx+numMatched) == len(seq) {
						continue
					}

					// If the edge doesn't match, there is no use in trying to match it at
					// other positions.
					return false
				}

				// If this is Ellipsis we consume it, and try matching the rest of the
				// matchers against the rest of the sequence at every offset.
				if matcher == Ellipsis {
					for startIdx := seqOffset + numMatched; startIdx < len(seq)-minSlots[matcherIdx+1]; startIdx++ {
						if cachedMatchesSeq(matcherIdx+1, startIdx) {
							return true
						}
					}
					return false
				}

				if !matcher.Matches(seq[seqIdx+numMatched]) {
					matches = false
					break
				}
				numMatched++
			}
			if matches {
				return true
			}
		}
		return false
	}

	// Since we have Ellipsis which can match any number of positions, including
	// zero, we memoize _matches_seq to avoid doing duplicate checks. This caps
	// the runtime of this matcher at O(len(matchers) * len(seq)); Otherwise this
	// would be quadratic on seq.
	type cacheKey struct {
		pOffset   int
		seqOffset int
	}
	cache := map[cacheKey]bool{}

	cachedMatchesSeq = func(pOffset, seqOffset int) bool {
		key := cacheKey{pOffset, seqOffset}
		if ret, ok := cache[key]; ok {
			return ret
		}
		ret := matchesPattern(pOffset, seqOffset)
		cache[key] = ret
		return ret
	}

	return cachedMatchesSeq(0, 0)
}
