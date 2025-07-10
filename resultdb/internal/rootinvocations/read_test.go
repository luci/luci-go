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

package rootinvocations

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestReadFunctions(t *testing.T) {
	ftt.Run("Read functions", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		const realm = "testproject:testrealm"
		const id = ID("root-inv-id")
		testData := NewBuilder(id).WithRealm(realm).Build()
		testutil.MustApply(ctx, t, InsertForTesting(testData)...)

		t.Run("Read", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				row, err := Read(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, row, should.Match(&testData))
			})

			t.Run("not found", func(t *ftt.Test) {
				_, err := Read(span.Single(ctx), "non-existent-id")
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
			})
		})

		t.Run("ReadRealm", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				r, err := ReadRealm(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, r, should.Equal(realm))
			})

			t.Run("not found", func(t *ftt.Test) {
				_, err := ReadRealm(span.Single(ctx), "non-existent-id")
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring("rootInvocations/non-existent-id not found"))
			})

			t.Run("empty ID", func(t *ftt.Test) {
				_, err := ReadRealm(span.Single(ctx), "")
				assert.That(t, err, should.ErrLike("id is unspecified"))
			})
		})

		t.Run("ReadRealmFromShard", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				shardID := ShardID{RootInvocationID: id, ShardIndex: 5}
				r, err := ReadRealmFromShard(span.Single(ctx), shardID)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, r, should.Equal(realm))
			})

			t.Run("not found", func(t *ftt.Test) {
				shardID := ShardID{RootInvocationID: "non-existent-id", ShardIndex: 0}
				_, err := ReadRealmFromShard(span.Single(ctx), shardID)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring("rootInvocations/non-existent-id not found"))
			})

			t.Run("empty ID", func(t *ftt.Test) {
				shardID := ShardID{RootInvocationID: "", ShardIndex: 0}
				_, err := ReadRealmFromShard(span.Single(ctx), shardID)
				assert.That(t, err, should.ErrLike("root invocation id is unspecified"))
			})
		})
	})
}
