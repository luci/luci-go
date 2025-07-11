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

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestReadFunctions(t *testing.T) {
	ftt.Run("Read functions", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")

		// Insert a root invocation.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build()
		ms := rootinvocations.InsertForTesting(rootInv)

		// Insert a work unit with all fields set.
		testData := NewBuilder(rootInvID, "work-unit-id").WithRealm("testproject:wu1").Build()
		ms = append(ms, InsertForTesting(testData)...)
		id := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id",
		}

		// Insert a work unit with minimal fields set.
		testDataMinimal := NewBuilder(rootInvID, "work-unit-id-minimal").WithRealm("testproject:wu1").WithMinimalFields().Build()
		idMinimal := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id-minimal",
		}
		ms = append(ms, InsertForTesting(testDataMinimal)...)

		testutil.MustApply(ctx, t, ms...)

		t.Run("Read", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				t.Run("mask: all fields", func(t *ftt.Test) {
					row, err := Read(span.Single(ctx), id, AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, row, should.Match(testData))
				})
				t.Run("mask: exclude extended properties", func(t *ftt.Test) {
					assert.Loosely(t, testData.ExtendedProperties, should.NotBeNil)
					testData.ExtendedProperties = nil

					row, err := Read(span.Single(ctx), id, ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, row, should.Match(testData))
				})
				t.Run("row: minimal fields", func(t *ftt.Test) {
					row, err := Read(span.Single(ctx), idMinimal, AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, row, should.Match(testDataMinimal))
				})
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, err := Read(span.Single(ctx), nonExistentID, AllFields)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike("rootInvocations/root-inv-id/workUnits/non-existent-id not found"))
			})
			t.Run("empty root invocation ID", func(t *ftt.Test) {
				id.RootInvocationID = ""
				_, err := Read(span.Single(ctx), id, AllFields)
				assert.That(t, err, should.ErrLike("rootInvocationID: unspecified"))
			})
			t.Run("empty work unit ID", func(t *ftt.Test) {
				id.WorkUnitID = ""
				_, err := Read(span.Single(ctx), id, AllFields)
				assert.That(t, err, should.ErrLike("workUnitID: unspecified"))
			})
		})
		t.Run("ReadBatch", func(t *ftt.Test) {
			// Insert additional root invocation and work units.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").WithRealm("testproject:root2").Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithRealm("testproject:wu21").Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithRealm("testproject:wu22").Build()
			ms := rootinvocations.InsertForTesting(rootInv2)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)
			testutil.MustApply(ctx, t, ms...)

			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id-minimal"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"}, // Duplicates are allowed.
				}
				t.Run("all fields", func(t *ftt.Test) {
					rows, err := ReadBatch(span.Single(ctx), ids, AllFields)
					assert.Loosely(t, err, should.BeNil)

					// Check that the returned rows match the expected data.
					assert.That(t, rows, should.Match([]*WorkUnitRow{
						testDataMinimal,
						testData,
						wu22,
						wu21,
						testData,
					}))
				})
				t.Run("exclude extended properties", func(t *ftt.Test) {
					rows, err := ReadBatch(span.Single(ctx), ids, ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)

					// Check that the returned rows match the inserted rows,
					// minus extended properties.
					expectedRows := []*WorkUnitRow{
						testDataMinimal.Clone(),
						testData.Clone(),
						wu22.Clone(),
						wu21.Clone(),
						testData.Clone(),
					}
					for _, r := range expectedRows {
						r.ExtendedProperties = nil
					}
					assert.That(t, rows, should.Match(expectedRows))
				})
			})
			t.Run("not found", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "non-existent-id"},
				}
				_, err := ReadBatch(span.Single(ctx), ids, AllFields)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike("rootInvocations/root-inv-id/workUnits/non-existent-id not found"))
			})
			t.Run("empty root invocation ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{WorkUnitID: "work-unit-id2"},
				}
				_, err := ReadBatch(span.Single(ctx), ids, AllFields)
				assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
			})
			t.Run("empty work unit ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID},
				}
				_, err := ReadBatch(span.Single(ctx), ids, AllFields)
				assert.That(t, err, should.ErrLike("ids[1]: workUnitID: unspecified"))
			})
		})

		t.Run("ReadRealm", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				r, err := ReadRealm(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, r, should.Equal("testproject:wu1"))
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
				assert.That(t, err, should.ErrLike("rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				_, err := ReadRealm(span.Single(ctx), ID{RootInvocationID: rootInvID})
				assert.That(t, err, should.ErrLike("workUnitID: unspecified"))
			})
		})
		t.Run("ReadRealms", func(t *ftt.Test) {
			// Insert an additional work unit in the existing root invocation.
			wu2 := NewBuilder(rootInvID, "work-unit-id2").WithRealm("testproject:wu2").Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with work units.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").WithRealm("testproject:root2").Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithRealm("testproject:wu21").Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithRealm("testproject:wu22").Build()
			ms = append(ms, rootinvocations.InsertForTesting(rootInv2)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)

			testutil.MustApply(ctx, t, ms...)

			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"}, // Duplicates are allowed.
				}
				realms, err := ReadRealms(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, realms, should.Match([]string{
					"testproject:wu1",
					"testproject:wu2",
					"testproject:wu22",
					"testproject:wu21",
					"testproject:wu2",
				}))
			})

			t.Run("not found", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "non-existent-id"},
				}
				_, err := ReadRealms(span.Single(ctx), ids)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring("rootInvocations/root-inv-id/workUnits/non-existent-id not found"))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{WorkUnitID: "work-unit-id2"},
				}
				_, err := ReadRealms(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID},
				}
				_, err := ReadRealms(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: workUnitID: unspecified"))
			})
		})
	})
}
