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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestReadFunctions(t *testing.T) {
	ftt.Run("Read functions", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")

		// Insert a root invocation with root work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build()
		ms := rootinvocations.InsertForTesting(rootInv)

		rootWU := NewBuilder(rootInvID, "root").WithRealm("testproject:root").Build()
		ms = append(ms, InsertForTesting(rootWU)...)

		// Insert a work unit with minimal fields set.
		testDataMinimal := NewBuilder(rootInvID, "work-unit-id-minimal").WithRealm("testproject:wu1").WithMinimalFields().Build()
		idMinimal := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id-minimal",
		}
		ms = append(ms, InsertForTesting(testDataMinimal)...)

		// Insert a work unit with all fields set.
		testData := NewBuilder(rootInvID, "work-unit-id").
			WithRealm("testproject:wu1").
			WithCreatedBy("test-user").
			WithCreateRequestID("test-request-id").
			WithFinalizationState(pb.WorkUnit_FINALIZED).
			Build()
		ms = append(ms, InsertForTesting(testData)...)

		// Update expected root work unit children accordingly.
		rootWU.ChildWorkUnits = []ID{
			{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
			{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id-minimal"},
		}

		id := ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id",
		}

		// Create a few child work units and invocations in the work unit.
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, "child1").WithParentWorkUnitID("work-unit-id").Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, "child2").WithParentWorkUnitID("work-unit-id").Build())...)
		ms = append(ms, InsertInvocationInclusionForTesting(id, "child-inv1")...)
		ms = append(ms, InsertInvocationInclusionForTesting(id, "child-inv2")...)

		// Update expected children accordingly.
		testData.ChildWorkUnits = []ID{
			{RootInvocationID: rootInvID, WorkUnitID: "child1"},
			{RootInvocationID: rootInvID, WorkUnitID: "child2"},
		}
		testData.ChildInvocations = []invocations.ID{
			"child-inv1",
			"child-inv2",
		}

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
					expected := testData.Clone()
					expected.ExtendedProperties = nil

					row, err := Read(span.Single(ctx), id, ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, row, should.Match(expected))
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
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
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
			wu2Root := NewBuilder("root-inv-id2", "root").WithRealm("testproject:root2").Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithRealm("testproject:wu21").Build()
			wu21Child1 := NewBuilder("root-inv-id2", "work-unit-id-child1").WithParentWorkUnitID("work-unit-id").Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithRealm("testproject:wu22").Build()

			ms := rootinvocations.InsertForTesting(rootInv2)
			ms = append(ms, InsertForTesting(wu2Root)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu21Child1)...)
			ms = append(ms, InsertForTesting(wu22)...)

			// Update expected root work unit children accordingly.
			wu21.ChildWorkUnits = []ID{{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id-child1"}}

			testutil.MustApply(ctx, t, ms...)

			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: rootInvID, WorkUnitID: "root"},
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
						rootWU,
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
						rootWU.Clone(),
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
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
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
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
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

		t.Run("ReadState", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				state, err := ReadFinalizationState(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, state, should.Equal(testData.FinalizationState))
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, err := ReadFinalizationState(span.Single(ctx), nonExistentID)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				_, err := ReadFinalizationState(span.Single(ctx), ID{WorkUnitID: "work-unit-id"})
				assert.That(t, err, should.ErrLike("rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				_, err := ReadFinalizationState(span.Single(ctx), ID{RootInvocationID: rootInvID})
				assert.That(t, err, should.ErrLike("workUnitID: unspecified"))
			})
		})

		t.Run("ReadRequestIDAndCreatedBy", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				requestID, createdBy, err := ReadRequestIDAndCreatedBy(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, requestID, should.Equal(testData.CreateRequestID))
				assert.That(t, createdBy, should.Equal(testData.CreatedBy))
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, _, err := ReadRequestIDAndCreatedBy(span.Single(ctx), nonExistentID)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				_, _, err := ReadRequestIDAndCreatedBy(span.Single(ctx), ID{WorkUnitID: "work-unit-id"})
				assert.That(t, err, should.ErrLike("rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				_, _, err := ReadRequestIDAndCreatedBy(span.Single(ctx), ID{RootInvocationID: rootInvID})
				assert.That(t, err, should.ErrLike("workUnitID: unspecified"))
			})
		})

		t.Run("ReadRealms", func(t *ftt.Test) {
			// Insert an additional work unit in the existing root invocation.
			wu2 := NewBuilder(rootInvID, "work-unit-id2").WithRealm("testproject:wu2").Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with work units.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").WithRealm("testproject:root2").Build()
			wuRoot := NewBuilder("root-inv-id2", "root").WithRealm("testproject:root2").Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithRealm("testproject:wu21").Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithRealm("testproject:wu22").Build()
			ms = append(ms, rootinvocations.InsertForTesting(rootInv2)...)
			ms = append(ms, InsertForTesting(wuRoot)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)

			testutil.MustApply(ctx, t, ms...)

			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: "root-inv-id2", WorkUnitID: "root"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"}, // Duplicates are allowed.
				}
				realms, err := ReadRealms(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, realms, should.Match(map[ID]string{
					wuRoot.ID:   "testproject:root2",
					wu21.ID:     "testproject:wu21",
					wu22.ID:     "testproject:wu22",
					wu2.ID:      "testproject:wu2",
					testData.ID: "testproject:wu1",
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
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
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

		t.Run("ReadStates", func(t *ftt.Test) {
			// Insert an additional work unit in the existing root invocation.
			wu2 := NewBuilder(rootInvID, "work-unit-id2").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with a work unit.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").Build()
			wuRoot := NewBuilder("root-inv-id2", "root").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
			ms = append(ms, rootinvocations.InsertForTesting(rootInv2)...)
			ms = append(ms, InsertForTesting(wuRoot)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)

			testutil.MustApply(ctx, t, ms...)
			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					{RootInvocationID: "root-inv-id2", WorkUnitID: "root"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"}, // Duplicates are allowed.
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id2"},
				}
				states, err := ReadFinalizationStates(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, states, should.Match([]pb.WorkUnit_FinalizationState{
					pb.WorkUnit_ACTIVE,
					pb.WorkUnit_FINALIZED,
					pb.WorkUnit_ACTIVE,
					pb.WorkUnit_FINALIZING,
					pb.WorkUnit_FINALIZED,
					pb.WorkUnit_FINALIZED,
				}))
			})

			t.Run("not found", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID, WorkUnitID: "non-existent-id"},
				}
				_, err := ReadFinalizationStates(span.Single(ctx), ids)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{WorkUnitID: "work-unit-id2"},
				}
				_, err := ReadFinalizationStates(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID},
				}
				_, err := ReadFinalizationStates(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: workUnitID: unspecified"))
			})
		})

		t.Run("ReadRequestIDsAndCreatedBys", func(t *ftt.Test) {
			// Insert an additional work unit in the existing root invocation.
			wu2 := NewBuilder(rootInvID, "work-unit-id2").
				WithCreateRequestID("req-wu2").
				WithCreatedBy("creator-wu2").
				Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with a work unit.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").Build()
			wuRoot := NewBuilder("root-inv-id2", "root").
				WithCreateRequestID("req-inv2-root").
				WithCreatedBy("creator-inv2-root").
				Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").
				WithCreateRequestID("req-inv2-wu").
				WithCreatedBy("creator-inv2-wu").
				Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").
				WithCreateRequestID("req-inv2-wu2").
				WithCreatedBy("creator-inv2-wu2").
				Build()
			ms = append(ms, rootinvocations.InsertForTesting(rootInv2)...)
			ms = append(ms, InsertForTesting(wuRoot)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)

			testutil.MustApply(ctx, t, ms...)

			t.Run("happy path with non-existent", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id"},
					{RootInvocationID: "root-inv-id2", WorkUnitID: "work-unit-id2"},
					{RootInvocationID: rootInvID, WorkUnitID: "non-existent-id"}, // Should not be found.
					{RootInvocationID: "root-inv-id2", WorkUnitID: "root"},
					{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"},
					id, // Duplicates are allowed.
				}
				results, err := ReadRequestIDsAndCreatedBys(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, results, should.Match([]*RequestIDAndCreatedBy{
					{
						RequestID: "test-request-id",
						CreatedBy: "test-user",
					},
					{
						RequestID: "req-inv2-wu",
						CreatedBy: "creator-inv2-wu",
					},
					{
						RequestID: "req-inv2-wu2",
						CreatedBy: "creator-inv2-wu2",
					},
					nil, // for non-existent-id
					{
						RequestID: "req-inv2-root",
						CreatedBy: "creator-inv2-root",
					},
					{
						RequestID: "req-wu2",
						CreatedBy: "creator-wu2",
					},
					{
						RequestID: "test-request-id",
						CreatedBy: "test-user",
					},
				}))

				// Validate results of the same id doesn't alias each other.
				results[0].CreatedBy = "mutated"
				assert.That(t, results[6].CreatedBy, should.Equal("test-user"))
			})
			t.Run("empty root invocation ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{WorkUnitID: "work-unit-id2"},
				}
				_, err := ReadRequestIDsAndCreatedBys(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID},
				}
				_, err := ReadRequestIDsAndCreatedBys(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: workUnitID: unspecified"))
			})
		})

		t.Run("ReadTestResultInfos", func(t *ftt.Test) {
			t.Run("happy path", func(t *ftt.Test) {
				// Create some further work units for testing.
				wu1 := NewBuilder(rootInvID, "content-1").
					WithFinalizationState(pb.WorkUnit_FINALIZED).
					WithRealm("testproject:realm-a").
					WithModuleID(&pb.ModuleIdentifier{
						ModuleName:    "module_name",
						ModuleScheme:  "module_scheme",
						ModuleVariant: pbutil.Variant("k", "v"),
					}).Build()
				wu1ID := ID{RootInvocationID: rootInvID, WorkUnitID: "content-1"}

				wu2 := NewBuilder(rootInvID, "content-2").
					WithFinalizationState(pb.WorkUnit_ACTIVE).
					WithRealm("testproject:realm-b").
					WithModuleID(nil).
					Build()
				wu2ID := ID{RootInvocationID: rootInvID, WorkUnitID: "content-2"}

				ms = InsertForTesting(wu1)
				ms = append(ms, InsertForTesting(wu2)...)
				testutil.MustApply(ctx, t, ms...)

				ids := []ID{wu1ID, wu2ID, wu1ID} // with duplicate
				results, err := ReadTestResultInfos(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)

				assert.That(t, results, should.Match(map[ID]TestResultInfo{
					wu1ID: {
						FinalizationState: pb.WorkUnit_FINALIZED,
						Realm:             "testproject:realm-a",
						ModuleID: &pb.ModuleIdentifier{
							ModuleName:        "module_name",
							ModuleScheme:      "module_scheme",
							ModuleVariant:     pbutil.Variant("k", "v"),
							ModuleVariantHash: "b1618cc2bf370a7c",
						},
					},
					wu2ID: {
						FinalizationState: pb.WorkUnit_ACTIVE,
						Realm:             "testproject:realm-b",
					},
				}))
			})

			t.Run("not found", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID, WorkUnitID: "non-existent-id"},
				}
				_, err := ReadTestResultInfos(span.Single(ctx), ids)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
			})

			t.Run("empty request", func(t *ftt.Test) {
				results, err := ReadTestResultInfos(span.Single(ctx), []ID{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.HaveLength(0))
			})
		})

		t.Run("ReadParents", func(t *ftt.Test) {
			// Insert an additional work unit in the existing root invocation.
			wu2 := NewBuilder(rootInvID, "work-unit-id2").WithParentWorkUnitID(rootWU.ID.WorkUnitID).Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with a work unit.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").Build()
			wu2Root := NewBuilder("root-inv-id2", "root").Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithParentWorkUnitID(wu2Root.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithParentWorkUnitID(wu21.ID.WorkUnitID).Build()
			ms = append(ms, rootinvocations.InsertForTesting(rootInv2)...)
			ms = append(ms, InsertForTesting(wu2Root)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)
			testutil.MustApply(ctx, t, ms...)
			t.Run("happy path", func(t *ftt.Test) {
				ids := []ID{
					wu2Root.ID,
					wu22.ID,
					wu22.ID, // Duplicate ID.
					rootWU.ID,
					wu21.ID,
					wu2Root.ID, // Duplicate ID.
					wu2.ID,
				}
				parents, err := ReadParents(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, parents, should.Match([]ID{
					{}, // root has no parent
					wu21.ID,
					wu21.ID,
					{},
					wu2Root.ID,
					{},
					rootWU.ID,
				}))
			})
			t.Run("empty request", func(t *ftt.Test) {
				results, err := ReadParents(span.Single(ctx), []ID{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, results, should.HaveLength(0))
			})
			t.Run("not found", func(t *ftt.Test) {
				ids := []ID{{RootInvocationID: rootInvID, WorkUnitID: "non-existent"}}
				_, err := ReadParents(span.Single(ctx), ids)
				assert.That(t, appstatus.Code(err), should.Equal(codes.NotFound))
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent" not found`))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				id.RootInvocationID = ""
				_, err := ReadParents(span.Single(ctx), []ID{id})
				assert.That(t, err, should.ErrLike("ids[0]: rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				id.WorkUnitID = ""
				_, err := ReadParents(span.Single(ctx), []ID{id})
				assert.That(t, err, should.ErrLike("ids[0]: workUnitID: unspecified"))
			})
		})
	})
}

func TestQueryFinalizerCandidates(t *testing.T) {
	ftt.Run("QueryFinalizerCandidates", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv")
		limit := 2
		t.Run("happy path", func(t *ftt.Test) {
			// Insert a root invocation with root work unit.
			rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build()
			rootWU := NewBuilder(rootInvID, "root").WithRealm("testproject:root").Build()

			// Insert two work units that is a finalizer candidate.
			wu1 := NewBuilder(rootInvID, "wu1").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()
			wu4 := NewBuilder(rootInvID, "wu4").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithFinalizerCandidateTime(time.Date(2025, time.October, 10, 13, 1, 2, 3, time.UTC)).
				Build()

			// Insert a work unit that is NOT a finalizer candidate (wrong state).
			wu2 := NewBuilder(rootInvID, "wu2").
				WithFinalizationState(pb.WorkUnit_ACTIVE).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()

			// Insert a work unit that is NOT a finalizer candidate (no candidate time).
			wu3 := NewBuilder(rootInvID, "wu3").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				Build()

			ct := testutil.MustApply(ctx, t, testutil.CombineMutations(
				rootinvocations.InsertForTesting(rootInv),
				InsertForTesting(rootWU),
				InsertForTesting(wu1),
				InsertForTesting(wu2),
				InsertForTesting(wu3),
				InsertForTesting(wu4),
			)...)

			t.Run("number of candidates below limit", func(t *ftt.Test) {
				candidates, err := QueryFinalizerCandidates(span.Single(ctx), rootInvID, limit)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, candidates, should.HaveLength(2))
				assert.Loosely(t, candidates[0].ID, should.Equal(wu1.ID))
				assert.Loosely(t, candidates[0].FinalizerCandidateTime, should.Match(ct))
				assert.Loosely(t, candidates[1].ID, should.Equal(wu4.ID))
				assert.Loosely(t, candidates[1].FinalizerCandidateTime, should.Match(wu4.FinalizerCandidateTime.Time))
			})

			t.Run("limit works", func(t *ftt.Test) {
				limit = 1
				candidates, err := QueryFinalizerCandidates(span.Single(ctx), rootInvID, limit)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, candidates, should.HaveLength(1))
				assert.Loosely(t, candidates[0].ID, should.Equal(wu1.ID))
				assert.Loosely(t, candidates[0].FinalizerCandidateTime, should.Match(ct))
			})
		})

		t.Run("root invocation doesn't exist", func(t *ftt.Test) {
			candidates, err := QueryFinalizerCandidates(span.Single(ctx), "non-existent", limit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, candidates, should.HaveLength(0))
		})
	})
}

func TestReadyToFinalize(t *testing.T) {
	ftt.Run("ReadyToFinalize", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("ready-to-finalize-inv")

		// Work units for testing.
		// Case 1: finalizing leaf work unit.
		wu1FinalizingLeaf := NewBuilder(rootInvID, "1").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()

		// Case 2: active leaf work unit.
		wu2ActiveLeaf := NewBuilder(rootInvID, "2").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

		// Case 3: Finalizing work unit with all children finalized.
		wu3Finalizing := NewBuilder(rootInvID, "3").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
		wu31Finalized := NewBuilder(rootInvID, "31").WithParentWorkUnitID(wu3Finalizing.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZED).Build()

		// Case 4: Finalizing work unit with active and finalized children.
		wu4Finalizing := NewBuilder(rootInvID, "4").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
		wu41Finalized := NewBuilder(rootInvID, "41").WithParentWorkUnitID(wu4Finalizing.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
		wu42Active := NewBuilder(rootInvID, "42").WithParentWorkUnitID(wu4Finalizing.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

		// Case 5: Finalizing work unit with finalizing and finalized children.
		wu5Finalizing := NewBuilder(rootInvID, "5").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
		wu51Finalized := NewBuilder(rootInvID, "51").WithParentWorkUnitID(wu5Finalizing.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
		wu52Finalizing := NewBuilder(rootInvID, "52").WithParentWorkUnitID(wu5Finalizing.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZING).Build()

		// Case 6: Active work unit with all children finalized.
		wu6Active := NewBuilder(rootInvID, "6").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		wu61Finalized := NewBuilder(rootInvID, "61").WithParentWorkUnitID(wu6Active.ID.WorkUnitID).WithFinalizationState(pb.WorkUnit_FINALIZED).Build()

		testutil.MustApply(ctx, t, testutil.CombineMutations(
			rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).Build()),
			InsertForTesting(NewBuilder(rootInvID, "root").Build()),
			InsertForTesting(wu1FinalizingLeaf),
			InsertForTesting(wu2ActiveLeaf),
			InsertForTesting(wu3Finalizing),
			InsertForTesting(wu31Finalized),
			InsertForTesting(wu4Finalizing),
			InsertForTesting(wu41Finalized),
			InsertForTesting(wu42Active),
			InsertForTesting(wu5Finalizing),
			InsertForTesting(wu51Finalized),
			InsertForTesting(wu52Finalizing),
			InsertForTesting(wu6Active),
			InsertForTesting(wu61Finalized),
		)...)
		ids := []ID{
			wu1FinalizingLeaf.ID, // ready
			wu2ActiveLeaf.ID,
			wu3Finalizing.ID, // ready
			wu31Finalized.ID,
			wu4Finalizing.ID,
			wu41Finalized.ID,
			wu42Active.ID,
			wu5Finalizing.ID,
			wu51Finalized.ID,
			wu52Finalizing.ID, // ready
			wu6Active.ID,
			wu61Finalized.ID,
		}

		t.Run("empty ids", func(t *ftt.Test) {
			readyIDs, err := ReadyToFinalize(span.Single(ctx), NewIDSet(), NewIDSet(wu1FinalizingLeaf.ID))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readyIDs, should.HaveLength(0))
		})
		t.Run("no ignore ids", func(t *ftt.Test) {
			readyIDs, err := ReadyToFinalize(span.Single(ctx), NewIDSet(ids...), NewIDSet())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readyIDs, should.HaveLength(3))
			assert.Loosely(t, readyIDs.Has(wu1FinalizingLeaf.ID), should.BeTrue)
			assert.Loosely(t, readyIDs.Has(wu3Finalizing.ID), should.BeTrue)
			assert.Loosely(t, readyIDs.Has(wu52Finalizing.ID), should.BeTrue)
		})
		t.Run("with ignore ids", func(t *ftt.Test) {
			readyIDs, err := ReadyToFinalize(span.Single(ctx), NewIDSet(ids...), NewIDSet(wu42Active.ID, wu4Finalizing.ID, wu61Finalized.ID))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readyIDs, should.HaveLength(4))
			assert.Loosely(t, readyIDs.Has(wu1FinalizingLeaf.ID), should.BeTrue)
			assert.Loosely(t, readyIDs.Has(wu3Finalizing.ID), should.BeTrue)
			assert.Loosely(t, readyIDs.Has(wu4Finalizing.ID), should.BeTrue) // Becomes ready because the active child is ignored.
			assert.Loosely(t, readyIDs.Has(wu52Finalizing.ID), should.BeTrue)
		})
	})
}

func TestWorkUnitUpdateRequests(t *testing.T) {
	ftt.Run("CheckWorkUnitUpdateRequestsExist", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")
		id := ID{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id"}
		id2 := ID{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id2"}
		id3 := ID{RootInvocationID: rootInvID, WorkUnitID: "work-unit-id3"}
		rootInvID2 := rootinvocations.ID("root-inv-id2")
		id20 := ID{RootInvocationID: rootInvID2, WorkUnitID: "work-unit-id20"}

		// Create a root invocation and work units.
		var ms []*spanner.Mutation
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID2).Build())...)

		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, "root").Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID2, "root").Build())...)

		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, id.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, id2.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, id3.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID2, id20.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build())...)

		updatedBy := "user:test@example.com"
		requestID := "request-123"

		// Insert requests.
		ms = append(ms, []*spanner.Mutation{
			InsertWorkUnitUpdateRequestForTesting(id, updatedBy, requestID),
			InsertWorkUnitUpdateRequestForTesting(id2, updatedBy, requestID),
			InsertWorkUnitUpdateRequestForTesting(id3, "user:another@example.com", requestID),
			InsertWorkUnitUpdateRequestForTesting(id3, updatedBy, "another-request-id"),
			InsertWorkUnitUpdateRequestForTesting(id20, updatedBy, requestID),
		}...)
		testutil.MustApply(ctx, t, ms...)

		t.Run("happy path", func(t *ftt.Test) {
			idsToQuery := []ID{
				id,
				id2,
				id3, // This one does not exist.
				id,  // Duplicates are allowed and should be handled.
				id20,
			}
			exists, err := CheckWorkUnitUpdateRequestsExist(span.Single(ctx), idsToQuery, updatedBy, requestID)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, exists, should.Match(map[ID]bool{
				id:   true,
				id2:  true,
				id20: true,
			}))
		})

		t.Run("empty ids slice", func(t *ftt.Test) {
			exists, err := CheckWorkUnitUpdateRequestsExist(span.Single(ctx), []ID{}, updatedBy, requestID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, exists, should.BeNil)
		})

		t.Run("empty root invocation ID", func(t *ftt.Test) {
			ids := []ID{
				id,
				{WorkUnitID: "work-unit-id2"},
			}
			_, err := CheckWorkUnitUpdateRequestsExist(span.Single(ctx), ids, updatedBy, requestID)
			assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
		})

		t.Run("empty work unit ID", func(t *ftt.Test) {
			ids := []ID{
				id,
				{RootInvocationID: rootInvID},
			}
			_, err := CheckWorkUnitUpdateRequestsExist(span.Single(ctx), ids, updatedBy, requestID)
			assert.That(t, err, should.ErrLike("ids[1]: workUnitID: unspecified"))
		})
	})
}

func TestReadDeadlineExpired(t *testing.T) {
	ftt.Run("ReadDeadlineExpired", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")
		rootInvID2 := rootinvocations.ID("root-inv-id2")

		// Create a root invocation and work units.
		var ms []*spanner.Mutation
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID2).Build())...)

		ms = append(ms, InsertForTesting(NewBuilder(rootInvID, "root").Build())...)
		ms = append(ms, InsertForTesting(NewBuilder(rootInvID2, "root").Build())...)

		now := time.Now()
		past := now.Add(-10 * time.Minute)
		future := now.Add(10 * time.Minute)

		wuExpired1 := NewBuilder(rootInvID, "expired1").WithDeadline(past).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		wuExpired2 := NewBuilder(rootInvID, "expired2").WithDeadline(past).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		wuUnexpired1 := NewBuilder(rootInvID, "unexpired1").WithDeadline(future).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		wuFinalized1 := NewBuilder(rootInvID, "finalized1").WithDeadline(past).WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
		wuExpiredOtherRoot := NewBuilder(rootInvID2, "expired_other_root").WithDeadline(past).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

		ms = append(ms, InsertForTesting(wuExpired1)...)
		ms = append(ms, InsertForTesting(wuExpired2)...)
		ms = append(ms, InsertForTesting(wuUnexpired1)...)
		ms = append(ms, InsertForTesting(wuFinalized1)...)

		ms = append(ms, InsertForTesting(wuExpiredOtherRoot)...)

		testutil.MustApply(ctx, t, ms...)

		// The sort order is based on the RootInvocationShardId.
		expectedEntries := []DeadlineExpiredEntry{
			{
				ID:             wuExpiredOtherRoot.ID,
				ActiveDeadline: wuExpiredOtherRoot.Deadline,
				Realm:          wuExpiredOtherRoot.Realm,
			},
			{
				ID:             wuExpired1.ID,
				ActiveDeadline: wuExpired1.Deadline,
				Realm:          wuExpired1.Realm,
			},
			{
				ID:             wuExpired2.ID,
				ActiveDeadline: wuExpired2.Deadline,
				Realm:          wuExpired2.Realm,
			},
		}
		t.Run("read expired work units", func(t *ftt.Test) {
			opts := ReadDeadlineExpiredOptions{
				ShardIndex: 0,
				ShardCount: 1,
				Limit:      100,
			}
			entries, err := ReadDeadlineExpired(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entries, should.Match(expectedEntries))
		})

		t.Run("limit works", func(t *ftt.Test) {
			opts := ReadDeadlineExpiredOptions{
				ShardIndex: 0,
				ShardCount: 1,
				Limit:      2,
			}
			entries, err := ReadDeadlineExpired(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entries, should.Match(expectedEntries[:2]))
		})

		t.Run("sharding works", func(t *ftt.Test) {
			opts := ReadDeadlineExpiredOptions{
				ShardIndex: 0,
				ShardCount: 2,
				Limit:      100,
			}
			entries, err := ReadDeadlineExpired(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entries, should.HaveLength(1))
			assert.Loosely(t, entries[0].ID, should.Equal(wuExpiredOtherRoot.ID))

			opts.ShardIndex = 1
			entries, err = ReadDeadlineExpired(span.Single(ctx), opts)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entries, should.HaveLength(2))
			assert.Loosely(t, entries[0].ID, should.Equal(wuExpired1.ID))
			assert.Loosely(t, entries[1].ID, should.Equal(wuExpired2.ID))
		})
	})
}

func TestRootInvocationShardShardKey(t *testing.T) {
	ftt.Run("rootInvocationShardShardKey", t, func(t *ftt.Test) {
		mustShard := func(i, count int) string {
			key, err := rootInvocationShardShardKey(i, count)
			assert.Loosely(t, err, should.BeNil)
			return key
		}
		t.Run("count of 1", func(t *ftt.Test) {
			assert.That(t, mustShard(0, 1), should.Equal(""))
			assert.That(t, mustShard(1, 1), should.Equal("ffffffff~"))
		})
		t.Run("count of 10", func(t *ftt.Test) {
			var keys []string
			for i := 0; i <= 10; i++ {
				keys = append(keys, mustShard(i, 10))
			}
			expectedKeys := []string{
				"",
				"19999998~",
				"33333332~",
				"4ccccccb~",
				"66666665~",
				"7fffffff~",
				"99999998~",
				"b3333332~",
				"cccccccb~",
				"e6666665~",
				"ffffffff~",
			}
			assert.That(t, keys, should.Match(expectedKeys))
		})
		t.Run("count of many", func(t *ftt.Test) {
			assert.That(t, mustShard(0, 1024), should.Equal(""))
			assert.That(t, mustShard(1, 1024), should.Equal("003fffff~"))
			assert.That(t, mustShard(512, 1024), should.Equal("7fffffff~"))
			assert.That(t, mustShard(1024, 1024), should.Equal("ffffffff~"))
		})
	})
}
