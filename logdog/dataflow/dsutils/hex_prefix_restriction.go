// Copyright 2024 The LUCI Authors.
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

package dsutils

import (
	"fmt"
	"math/big"
	"strings"

	"go.chromium.org/luci/common/errors"
)

const totalSlices int64 = 65536

// hexPrefixRestriction represents a string with hex prefix range between
// `Start` and `End`.
type hexPrefixRestriction struct {
	HexPrefixLength int

	StartIsExclusive bool
	Start            string

	EndIsExclusive bool
	EndIsUnbounded bool
	End            string
}

func (r *hexPrefixRestriction) RangeString() string {
	opening := "["
	if r.StartIsExclusive {
		opening = "("
	}
	closing := "]"
	if r.EndIsExclusive {
		closing = "]"
	}
	endString := r.End
	if r.EndIsUnbounded {
		endString = "∞"
	}
	return fmt.Sprintf("%s\"%s\", \"%s\"%s", opening, r.Start, endString, closing)
}

func (r *hexPrefixRestriction) EstimatedSize() float64 {
	total := float64(totalSlices)

	startPrefixInt, ok := hexPrefixToInt(r.Start, r.HexPrefixLength)
	if !ok {
		return total
	}
	start, _ := startPrefixInt.Float64()

	endCalcBase := r.End
	if r.EndIsUnbounded {
		// If the end is not bounded, use the maximum hex number with a length of
		// `hexPrefixLength` as the end to calculate the splitting point.
		endCalcBase = strings.Repeat("f", r.HexPrefixLength)
	}
	endPrefixInt, ok := hexPrefixToInt(endCalcBase, r.HexPrefixLength)
	if !ok {
		return total
	}
	end, _ := endPrefixInt.Float64()

	allBigInt, ok := hexPrefixToInt(strings.Repeat("f", r.HexPrefixLength), r.HexPrefixLength)
	if !ok {
		panic("total should always be a valid hex")
	}
	all, _ := allBigInt.Float64()

	return (end - start) / all * total
}

type hexPrefixRestrictionTracker struct {
	// restriction is the restriction that the tracker is tracking.
	restriction hexPrefixRestriction
	// hasClaimed indicates whether there were any claim made.
	hasClaimed bool
	// claimed is the last claimed item, if any.
	claimed string
	// claimedAll indicates whether everything has been claimed.
	// This is a cheap way of marking restriction as finished.
	claimedAll bool
	// madeEndingClaim indicates whether a claim that exhausted the restriction
	// has been made.
	madeEndingClaim bool
	// err stores the error that may happen during various operations.
	err error
}

func newHexPrefixRestrictionTracker(rest hexPrefixRestriction) *hexPrefixRestrictionTracker {
	emptyRange := !rest.EndIsUnbounded &&
		(rest.End < rest.Start || (rest.Start == rest.End && (rest.StartIsExclusive || rest.EndIsExclusive)))

	return &hexPrefixRestrictionTracker{
		restriction:     rest,
		hasClaimed:      false,
		claimed:         "",
		claimedAll:      emptyRange,
		madeEndingClaim: false,
		err:             nil,
	}
}

type HexPosClaim struct {
	Value string
	End   bool
}

// TryClaim implements `sdf.RTracker`.
//
// TryClaim accepts a `string` presenting the end key of a block of
// work. It successfully claims it if the key is greater than the previously
// claimed key and within the restriction. Claiming a key at the end of the
// restriction signals that the entire restriction has been processed and is now
// done, at which point this method signals to end processing.
//
// The tracker stops with an error if a claim is attempted after the tracker has
// signalled to stop, if a key is claimed is outside of the restriction, or if a
// key is claimed less than or equals to a previously claimed key.
func (t *hexPrefixRestrictionTracker) TryClaim(rawKey any) bool {
	if t.err != nil {
		return false
	}

	key := rawKey.(HexPosClaim)
	if key.End && key.Value != "" {
		t.err = errors.Reason("cannot End and Value cannot be specified at the same time").Err()
		return false
	}

	if key.End {
		t.claimedAll = true
		t.madeEndingClaim = true
		return false
	}

	if t.madeEndingClaim {
		// Claiming after the a failed claim is an indication of error on the caller
		// side. Make it fail fast so debugging can be easier.
		t.err = errors.Reason("cannot claim %s after the everything has been claimed in range %s", key.Value, t.restriction.RangeString()).Err()
		return false
	}

	value := key.Value

	beforeStart := value < t.restriction.Start ||
		t.restriction.StartIsExclusive && value <= t.restriction.Start

	// The key must be within the range of the restriction.
	if beforeStart {
		t.err = errors.Reason("cannot claim a key `%s` out of bounds of the restriction %s", value, t.restriction.RangeString()).Err()
		return false
	}

	// A claimed key cannot be claimed again.
	if t.hasClaimed && value <= t.claimed {
		t.err = errors.Reason("cannot claim a key `%s` smaller than the previously claimed key `%s`", value, t.claimed).Err()
		return false
	}

	afterEnd := !t.restriction.EndIsUnbounded &&
		(value > t.restriction.End || t.restriction.EndIsExclusive && value >= t.restriction.End)
	// The restriction might have been shrink when the worker was running,
	// therefore causing the worker trying to claim a key larger than the end
	// boundary. We should stop the restriction tracker and ask the worker to stop
	// going further.
	if afterEnd {
		t.claimedAll = true
		t.madeEndingClaim = true
		return false
	}

	t.hasClaimed = true
	t.claimed = value

	return true
}

// GetError implements `sdf.RTracker`.
func (t *hexPrefixRestrictionTracker) GetError() error {
	return t.err
}

// TrySplit implements `sdf.RTracker`.
func (t *hexPrefixRestrictionTracker) TrySplit(fraction float64) (primary, residual any, err error) {
	if t.err != nil || t.IsDone() {
		return t.restriction, nil, nil
	}

	// Translate `fraction` to `primarySliceCount` so doing calculation on it is
	// easier. `primarySliceCount == x` means `x` out of `totalSlices` equally
	// divided slices should be in the primary split.
	primarySliceCount := int64(fraction * float64(totalSlices))
	if primarySliceCount < 0 {
		primarySliceCount = 0
	} else if primarySliceCount > totalSlices {
		primarySliceCount = totalSlices
	}

	if primarySliceCount == 0 {
		return t.trySelfCheckPointingSplit()
	}

	hexPrefixLength := t.restriction.HexPrefixLength

	// Get the start of the remaining portion.
	remainingStartIsExclusive := t.restriction.StartIsExclusive
	remainingStart := t.restriction.Start
	// If something has been claimed, the start boundary should move to the
	// claimed key.
	if t.hasClaimed {
		remainingStartIsExclusive = true
		remainingStart = t.claimed
	}

	// Get end of the raining portion, which is also the end of the original
	// restriction.
	endIsExclusive := t.restriction.EndIsExclusive
	endIsUnbounded := t.restriction.EndIsUnbounded
	end := t.restriction.End

	// Convert remaining start to a number so we can use it to calculate the
	// splitting point.
	remainingStartPrefixInt, ok := hexPrefixToInt(remainingStart, hexPrefixLength)
	// If the boundary is does not have a valid hex prefix somehow, just give up
	// and use the original restriction. We don't need to fail the whole bundle.
	if !ok {
		return t.restriction, nil, nil
	}

	// Convert end to a number so we can use it to calculate the splitting point.
	endCalcBase := end
	if endIsUnbounded {
		// If the end is not bounded, use the maximum hex number with a length of
		// `hexPrefixLength` as the end to calculate the splitting point.
		endCalcBase = strings.Repeat("f", hexPrefixLength)
	}
	endPrefixInt, ok := hexPrefixToInt(endCalcBase, hexPrefixLength)
	// If the boundary is does not have a valid hex prefix somehow, just give up
	// and use the original restriction. We don't need to fail the whole bundle.
	if !ok {
		return t.restriction, nil, nil
	}

	// Not sure if all of the bigint operation are atomic. Do not reuse bigInts to
	// store temporary results. Splitting point is calculated as:
	// `splitPoint = start + (end - start) * (primarySlices / totalSlices)`.
	remainingSize := big.NewInt(0)
	remainingSize = remainingSize.Sub(endPrefixInt, remainingStartPrefixInt)
	// Do multiplication first to reduce precision lost due to integer division.
	expandedSize := big.NewInt(0)
	expandedSize = expandedSize.Mul(remainingSize, big.NewInt(primarySliceCount))
	primarySize := big.NewInt(0)
	primarySize = primarySize.Div(expandedSize, big.NewInt(totalSlices))
	splitPointInt := big.NewInt(0)
	splitPointInt = splitPointInt.Add(remainingStartPrefixInt, primarySize)

	// Convert the splitting point to hex and check if its in the bound of the
	// remaining start/end.
	splitPointHex := fmt.Sprintf("%0*x", hexPrefixLength, splitPointInt)
	splitIsBeforeRemainingStart := splitPointHex < remainingStart ||
		remainingStartIsExclusive && splitPointHex <= remainingStart
	if splitIsBeforeRemainingStart {
		return t.trySelfCheckPointingSplit()
	}
	splitIsAfterEnd := !endIsUnbounded && (splitPointHex > end ||
		endIsExclusive && splitPointHex >= end)
	if splitIsAfterEnd {
		return t.tryNoopSplit()
	}

	// Split into `(original start, splitting point]`, and
	// `(splitting point, original end)`
	t.restriction.EndIsUnbounded = false
	t.restriction.End = splitPointHex
	t.restriction.EndIsExclusive = false
	return t.restriction, hexPrefixRestriction{
		HexPrefixLength:  hexPrefixLength,
		StartIsExclusive: true,
		Start:            splitPointHex,
		EndIsUnbounded:   endIsUnbounded,
		EndIsExclusive:   endIsExclusive,
		End:              end,
	}, nil
}

// tryNoopSplit performs a noop split.
func (t *hexPrefixRestrictionTracker) tryNoopSplit() (primary, residual any, err error) {
	return t.restriction, nil, nil
}

// trySelfCheckPointingSplit performs a self-checkpointing split, shrink the
// primary split to contain all the finished portion and return the remainder
// as a new split.
func (t *hexPrefixRestrictionTracker) trySelfCheckPointingSplit() (primary, residual any, err error) {
	newHexPrefixLength := t.restriction.HexPrefixLength
	newStartIsExclusive := t.restriction.StartIsExclusive
	newStart := t.restriction.Start
	newEndIsUnbounded := t.restriction.EndIsUnbounded
	newEndIsExclusive := t.restriction.EndIsExclusive
	newEnd := t.restriction.End
	if t.hasClaimed {
		newStartIsExclusive = true
		newStart = t.claimed
	}

	// Shrink to current restriction.
	if t.hasClaimed {
		// Shrink the current restriction's end to the claimed key if a key was
		// claimed.
		t.restriction.EndIsExclusive = false
		t.restriction.End = t.claimed
	} else {
		// Shrink the current restriction's start to the end if end is specified.
		t.restriction.EndIsExclusive = true
		t.restriction.End = t.restriction.Start
	}
	t.restriction.EndIsUnbounded = false
	t.claimedAll = true

	return t.restriction, hexPrefixRestriction{
		HexPrefixLength:  newHexPrefixLength,
		StartIsExclusive: newStartIsExclusive,
		Start:            newStart,
		EndIsUnbounded:   newEndIsUnbounded,
		EndIsExclusive:   newEndIsExclusive,
		End:              newEnd,
	}, nil
}

// GetProgress implements `sdf.RTracker`.
func (t *hexPrefixRestrictionTracker) GetProgress() (float64, float64) {
	total := float64(totalSlices)
	if t.claimedAll {
		return total, 0
	}

	if !t.hasClaimed {
		return 0, total
	}

	startPrefixInt, ok := hexPrefixToInt(t.restriction.Start, t.restriction.HexPrefixLength)
	if !ok {
		return 0, total
	}
	start, _ := startPrefixInt.Float64()

	endCalcBase := t.restriction.End
	if t.restriction.EndIsUnbounded {
		// If the end is not bounded, use the maximum hex number with a length of
		// `hexPrefixLength` as the end to calculate the splitting point.
		endCalcBase = strings.Repeat("f", t.restriction.HexPrefixLength)
	}
	endPrefixInt, ok := hexPrefixToInt(endCalcBase, t.restriction.HexPrefixLength)
	if !ok {
		return 0, total
	}
	end, _ := endPrefixInt.Float64()

	claimedPrefixInt, ok := hexPrefixToInt(t.claimed, t.restriction.HexPrefixLength)
	if !ok {
		return 0, total
	}
	claimed, _ := claimedPrefixInt.Float64()

	progress := (claimed - start) / (end - start) * total
	return progress, total - (progress)
}

// IsDone implements `sdf.RTracker`.
func (t *hexPrefixRestrictionTracker) IsDone() bool {
	if t.err != nil {
		return false
	}

	return t.claimedAll
}

// GetRestriction implements `sdf.RTracker`.
func (t *hexPrefixRestrictionTracker) GetRestriction() any {
	return t.restriction
}

// IsBounded implements `sdf.BoundableRTracker`.
func (t *hexPrefixRestrictionTracker) IsBounded() bool {
	// Even if end is unbounded, there's still a limited number of entities in the
	// database.
	return true
}

func hexPrefixToInt(v string, prefixLen int) (*big.Int, bool) {
	prefix := v[:min(len(v), prefixLen)]
	// Normalized the prefix hex to the specified length
	prefix += strings.Repeat("0", max(0, prefixLen-len(prefix)))

	prefixInt := big.NewInt(0)
	return prefixInt.SetString(prefix, 16)
}
