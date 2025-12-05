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

package testresultsv2

import (
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestCreate(t *testing.T) {
	ftt.Run("Create", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		// Create the root invocation and shards.
		ri := rootinvocations.NewBuilder("root-inv-id").WithRealm("testproject:testrealm").Build()

		rootInvShard := rootinvocations.ShardID{
			RootInvocationID: "root-inv-id",
			ShardIndex:       1,
		}

		testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(ri)...)

		trBuilder := NewBuilder().
			WithRootInvocationShardID(rootInvShard).
			WithResultID("result-1")

		verifyCreate := func(t *ftt.Test, tr *TestResultRow) {
			commitTimestamp := testutil.MustApply(ctx, t, Create(tr))
			// Build again to avoid any side effects of Create on `tr`.
			expectedRow := trBuilder.Build()
			expectedRow.CreateTime = commitTimestamp

			// Read it back.
			rows, err := ReadAllForTesting(span.Single(ctx))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Match([]*TestResultRow{expectedRow}))
		}

		t.Run("Maximimal fields", func(t *ftt.Test) {
			tr := trBuilder.Build()
			verifyCreate(t, tr)
		})
		t.Run("Minimal fields", func(t *ftt.Test) {
			tr := trBuilder.WithMinimalFields().Build()
			verifyCreate(t, tr)
		})
		t.Run("Test metadata", func(t *ftt.Test) {
			t.Run("Name only", func(t *ftt.Test) {
				tr := trBuilder.WithTestMetadata(&pb.TestMetadata{Name: "test-name"}).Build()
				verifyCreate(t, tr)
			})
			t.Run("Location only", func(t *ftt.Test) {
				tr := trBuilder.WithTestMetadata(&pb.TestMetadata{Location: &pb.TestLocation{
					Repo:     "https://chromium.googlesource.com/chromium/src",
					FileName: "some/folder/to/file_name.java",
					Line:     123,
				}}).Build()
				verifyCreate(t, tr)
			})
		})
	})
}
