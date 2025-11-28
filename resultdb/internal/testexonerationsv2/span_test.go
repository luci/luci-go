// Copyright 2025 The LUCI Authors.
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

package testexonerationsv2

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestSpan(t *testing.T) {
	ftt.Run("TestSpan", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run("Create", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("root-inv")
			rootInvRow := rootinvocations.NewBuilder(rootInvID).Build()
			rootInvMutations := rootinvocations.InsertForTesting(rootInvRow)

			shardID := rootinvocations.ShardID{RootInvocationID: rootInvID, ShardIndex: 0}
			expectedRow := NewBuilder().
				WithRootInvocationShardID(shardID).
				Build()

			mutations := append(rootInvMutations, Create(expectedRow))
			commitTime, err := span.Apply(ctx, mutations)
			assert.Loosely(t, err, should.BeNil)

			rows, err := ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.HaveLength(1))
			expectedRow.CreateTime = commitTime
			assert.Loosely(t, rows[0], should.Match(expectedRow))
		})
	})
}
