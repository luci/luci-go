// Copyright 2020 The LUCI Authors.
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
	"sort"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

// EquivalentPatchsetRange computes range of patchsets code-wise equivalent to
// the current patchset.
//
// Gerrit categorises each new patchset (aka Revision) according to difference
// from prior one. The rebases are counted as equivalent, even though
// dependencies may have changed. Thus, only REWORK changes code.
//
// Generally, all patchsets are numbered 1,2,3,...n without gaps. But this
// function doesn't assume this, thus Gerrit might potentially support wiping
// out individual patchsets, creating gaps without affecting CV.
func EquivalentPatchsetRange(info *gerritpb.ChangeInfo) (minEquiPatchset, currentPatchset int, err error) {
	if len(info.Revisions) == 0 {
		err = errors.Reason("ChangeInfo must have all revisions populated").Err()
		return
	}
	revs := make([]*gerritpb.RevisionInfo, 0, len(info.Revisions))
	for _, rev := range info.Revisions {
		revs = append(revs, rev)
	}
	sort.Slice(revs, func(i, j int) bool {
		return revs[i].Number > revs[j].Number // largest patchset first.
	})

	// Validate ChangeInfo to avoid problems later.
	switch rev, ok := info.Revisions[info.CurrentRevision]; {
	case !ok:
		err = errors.Reason("ChangeInfo must have current_revision populated").Err()
		return
	case rev != revs[0]:
		err = errors.Reason("ChangeInfo.currentPatchset %v doesn't have largest patchset %v", rev, revs[0]).Err()
		return
	}

	currentPatchset = int(revs[0].Number)
	minEquiPatchset = currentPatchset
	for i, rev := range revs[:len(revs)-1] {
		switch rev.Kind {
		case gerritpb.RevisionInfo_REWORK:
			return
		case gerritpb.RevisionInfo_NO_CHANGE,
			gerritpb.RevisionInfo_NO_CODE_CHANGE,
			gerritpb.RevisionInfo_MERGE_FIRST_PARENT_UPDATE,
			gerritpb.RevisionInfo_TRIVIAL_REBASE,
			gerritpb.RevisionInfo_TRIVIAL_REBASE_WITH_MESSAGE_UPDATE:
			minEquiPatchset = int(revs[i+1].Number)
		default:
			err = errors.Reason("Unknown revision kind %d %s ps#%d",
				rev.Kind, rev.Kind, rev.GetNumber()).Err()
			return
		}
	}
	return
}
