// Copyright 2026 The LUCI Authors.
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

package artifacts

import (
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

func TestFilterHasArtifacts(t *testing.T) {
	ftt.Run(`FilterHasArtifacts`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		invID1 := invocations.ID("inv1")
		invID2 := invocations.ID("inv2")
		invID3 := invocations.ID("inv3")

		testutil.MustApply(ctx, t,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, nil),
			insert.Artifact("inv1", "", "a", nil),
			insert.Invocation("inv2", pb.Invocation_ACTIVE, nil),
			insert.Invocation("inv3", pb.Invocation_ACTIVE, nil),
			insert.Artifact("inv3", "", "a", nil),
		)

		t.Run(`Returns subset of invocations with artifacts`, func(t *ftt.Test) {
			invIDs := invocations.NewIDSet(invID1, invID2, invID3)
			hasArtifacts, err := FilterHasArtifacts(span.Single(ctx), invIDs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasArtifacts, should.Match(invocations.NewIDSet(invID1, invID3)))
		})

		t.Run(`Returns empty set if no artifacts exist`, func(t *ftt.Test) {
			invIDs := invocations.NewIDSet(invID2)
			hasArtifacts, err := FilterHasArtifacts(span.Single(ctx), invIDs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasArtifacts, should.BeEmpty)
		})

		t.Run(`Returns empty set if input is empty`, func(t *ftt.Test) {
			hasArtifacts, err := FilterHasArtifacts(span.Single(ctx), invocations.NewIDSet())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasArtifacts, should.BeEmpty)
		})
	})
}

func TestListFuzzyMatches(t *testing.T) {
	ftt.Run(`ListFuzzyMatches`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		invID := invocations.ID("inv")
		parentID := "tr/t/r"

		testutil.MustApply(ctx, t,
			insert.Invocation(invID, pb.Invocation_ACTIVE, nil),
			insert.Artifact(invID, parentID, "adb-connect-logs_123.txt", nil),
			insert.Artifact(invID, parentID, "adb-connect-logs_456.txt", nil),
			insert.Artifact(invID, parentID, "other-log.txt", nil),
		)

		t.Run(`Matches with digits removed`, func(t *ftt.Test) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			arts, err := ListFuzzyMatches(ctx, invID, parentID, "adb-connect-logs_789.txt", 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, arts, should.HaveLength(2))
			ids := []string{arts[0].ArtifactId, arts[1].ArtifactId}
			sort.Strings(ids)
			assert.Loosely(t, ids, should.Match([]string{"adb-connect-logs_123.txt", "adb-connect-logs_456.txt"}))
		})

		t.Run(`Does not match different base names`, func(t *ftt.Test) {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			arts, err := ListFuzzyMatches(ctx, invID, parentID, "different-log.txt", 10)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, arts, should.BeEmpty)
		})
	})
}
