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
	"encoding/hex"
	"testing"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestEquivalentPatchsetRange(t *testing.T) {
	t.Parallel()

	ftt.Run("EquivalentPatchsetRange", t, func(t *ftt.Test) {
		t.Run("No revisions", func(t *ftt.Test) {
			_, _, err := EquivalentPatchsetRange(makeCI())
			assert.ErrIsLike(t, err, "must have all revisions populated")
		})

		t.Run("Wrong CurrentRevision", func(t *ftt.Test) {
			ci := makeCI(gerritpb.RevisionInfo_REWORK)
			ci.CurrentRevision = ""
			_, _, err := EquivalentPatchsetRange(ci)
			assert.ErrIsLike(t, err, "must have current_revision populated")
		})

		t.Run("Wrong Kind", func(t *ftt.Test) {
			_, _, err := EquivalentPatchsetRange(makeCI(
				gerritpb.RevisionInfo_TRIVIAL_REBASE,
				gerritpb.RevisionInfo_Kind(199)))
			assert.ErrIsLike(t, err, "Unknown revision kind 199")
		})

		t.Run("works", func(t *ftt.Test) {
			m, p, err := EquivalentPatchsetRange(makeCI(gerritpb.RevisionInfo_REWORK))
			assert.NoErr(t, err)
			assert.Loosely(t, m, should.Equal(1))
			assert.Loosely(t, p, should.Equal(1))

			m, p, err = EquivalentPatchsetRange(makeCI(
				gerritpb.RevisionInfo_REWORK,
				gerritpb.RevisionInfo_TRIVIAL_REBASE,
				gerritpb.RevisionInfo_REWORK,
				gerritpb.RevisionInfo_NO_CHANGE,
				gerritpb.RevisionInfo_NO_CODE_CHANGE,
				gerritpb.RevisionInfo_MERGE_FIRST_PARENT_UPDATE,
				gerritpb.RevisionInfo_TRIVIAL_REBASE,
				gerritpb.RevisionInfo_TRIVIAL_REBASE_WITH_MESSAGE_UPDATE,
			))
			assert.NoErr(t, err)
			assert.Loosely(t, m, should.Equal(3))
			assert.Loosely(t, p, should.Equal(8))
		})
	})
}

func makeCI(kinds ...gerritpb.RevisionInfo_Kind) *gerritpb.ChangeInfo {
	ci := &gerritpb.ChangeInfo{
		Revisions: make(map[string]*gerritpb.RevisionInfo, len(kinds)),
	}
	var rev string
	for i, k := range kinds {
		rev = hex.EncodeToString([]byte{byte(i ^ 37), byte(k ^ 7)})
		ci.Revisions[rev] = &gerritpb.RevisionInfo{Number: int32(i + 1), Kind: k}
	}
	ci.CurrentRevision = rev
	return ci
}
