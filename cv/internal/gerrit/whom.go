// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"fmt"
	"sort"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// Whoms is a collection of `Whom`s.
type Whoms []Whom

// Dedupe removes duplicates and sorts the whoms in-place.
func (whoms Whoms) Dedupe() {
	if len(whoms) <= 1 {
		return
	}
	whomMap := make(map[Whom]struct{}, len(whoms))
	for _, w := range whoms {
		whomMap[w] = struct{}{}
	}
	whoms = whoms[:0]
	for w := range whomMap {
		whoms = append(whoms, w)
	}
	sort.Slice(whoms, func(i, j int) bool {
		return whoms[i] < whoms[j]
	})
}

// ToAccountIDsSorted translates whom to actual Gerrit account ids in a CL.
func (whoms Whoms) ToAccountIDsSorted(ci *gerritpb.ChangeInfo) []int64 {
	accountSet := make(map[int64]struct{})
	for _, whom := range whoms {
		switch whom {
		case Whom_OWNER:
			accountSet[ci.GetOwner().GetAccountId()] = struct{}{}
		case Whom_REVIEWERS:
			for _, reviewer := range ci.GetReviewers().GetReviewers() {
				accountSet[reviewer.GetAccountId()] = struct{}{}
			}
		case Whom_CQ_VOTERS:
			for _, vote := range ci.GetLabels()["Commit-Queue"].GetAll() {
				if vote.GetValue() > 0 {
					accountSet[vote.GetUser().GetAccountId()] = struct{}{}
				}
			}
		case Whom_PS_UPLOADER:
			accountSet[ci.GetRevisions()[ci.GetCurrentRevision()].GetUploader().GetAccountId()] = struct{}{}
		default:
			panic(fmt.Sprintf("unknown whom: %s", whom))
		}
	}
	ret := make([]int64, 0, len(accountSet))
	for acct := range accountSet {
		ret = append(ret, acct)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	return ret
}
