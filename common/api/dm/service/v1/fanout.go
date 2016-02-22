// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"errors"
	"sort"

	"github.com/xtgo/set"
)

// Normalize sorts and uniq's attempt nums
func (a *AttemptFanout) Normalize() error {
	for q, vals := range a.To {
		if vals == nil {
			vals = &AttemptFanout_AttemptNums{}
			a.To[q] = vals
		} else {
			if err := vals.Normalize(); err != nil {
				return err
			}
		}
	}
	return nil
}

type revUint32Slice []uint32

func (u revUint32Slice) Len() int      { return len(u) }
func (u revUint32Slice) Swap(i, j int) { u[i], u[j] = u[j], u[i] }

// Less intentionally reverses the comparison of i and j.
func (u revUint32Slice) Less(i, j int) bool { return u[j] < u[i] }

// Normalize sorts and uniq's attempt nums. If Nums equals [0], [] or nil,
// it implies all attempts for the quest and will be normalized to nil.
//
// It is an error for Nums to contain 0 as well as other numbers.
func (a *AttemptFanout_AttemptNums) Normalize() error {
	if len(a.Nums) == 0 || (len(a.Nums) == 1 && a.Nums[0] == 0) {
		a.Nums = nil
		return nil
	}
	slc := revUint32Slice(a.Nums)
	sort.Sort(slc)
	if a.Nums[len(a.Nums)-1] == 0 {
		return errors.New("AttemptFanout.Nums contains 0 as well as other values.")
	}
	a.Nums = a.Nums[:set.Uniq(slc)]
	return nil
}

// NewAttemptFanout is a convenience method for making a normalized
// *AttemptFanout with a pre-normalized literal map of quest -> attempt nums.
//
// If the provided data is invalid, this method will panic.
func NewAttemptFanout(data map[string][]uint32) *AttemptFanout {
	ret := &AttemptFanout{To: make(map[string]*AttemptFanout_AttemptNums, len(data))}
	for qst, atmpts := range data {
		nums := &AttemptFanout_AttemptNums{Nums: atmpts}
		if err := nums.Normalize(); err != nil {
			panic(err)
		}
		ret.To[qst] = nums
	}
	return ret
}

// AddAIDs adds the given Attempt_ID to the AttemptFanout
func (a *AttemptFanout) AddAIDs(aids ...*Attempt_ID) {
	for _, aid := range aids {
		if a.To == nil {
			a.To = map[string]*AttemptFanout_AttemptNums{}
		}
		atmptNums := a.To[aid.Quest]
		if atmptNums == nil {
			atmptNums = &AttemptFanout_AttemptNums{}
			a.To[aid.Quest] = atmptNums
		}
		atmptNums.Nums = append(atmptNums.Nums, aid.Id)
	}
}
