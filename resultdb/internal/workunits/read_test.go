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

package workunits

import (
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestReadFunctions(t *testing.T) {
	ftt.Run("Read functions", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		const realm = "testproject:testrealm"
		rootInvID := rootinvocations.ID("root-inv-id")
		id := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id",
		}

		// Insert a root invocation.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(realm).Build()
		mutations := rootinvocations.InsertForTesting(rootInv)

		// Insert a work unit.
		testData := NewBuilder(rootInvID, "work-unit-id").WithRealm(realm).Build()
		mutations = append(mutations, InsertForTesting(testData)...)
		testutil.MustApply(ctx, t, mutations...)

		t.Run("Read", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				row, err := Read(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)

				// Read populates some fields that the builder doesn't.
				expected := testData
				expected.Instructions = instructionutil.InstructionsWithNames(expected.Instructions, id.Name())

				assert.That(t, row, should.Match(&expected))
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, err := Read(span.Single(ctx), nonExistentID)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike("rootInvocations/root-inv-id/workUnits/non-existent-id not found"))
			})
		})

		t.Run("ReadRealm", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				r, err := ReadRealm(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, r, should.Equal(realm))
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, err := ReadRealm(span.Single(ctx), nonExistentID)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring("rootInvocations/root-inv-id/workUnits/non-existent-id not found"))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				_, err := ReadRealm(span.Single(ctx), ID{WorkUnitID: "work-unit-id"})
				assert.That(t, err, should.ErrLike("rootInvocationID is unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				_, err := ReadRealm(span.Single(ctx), ID{RootInvocationID: rootInvID})
				assert.That(t, err, should.ErrLike("workUnitID is unspecified"))
			})
		})
	})
}
