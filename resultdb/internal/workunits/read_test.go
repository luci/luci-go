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

		// Insert a work unit with all fields set.
		testData := NewBuilder(rootInvID, "work-unit-id").
			WithRealm("testproject:wu1").
			WithCreatedBy("test-user").
			WithCreateRequestID("test-request-id").
			WithState(pb.WorkUnit_FINALIZED).
			Build()
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
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithRealm("testproject:wu22").Build()
			ms := rootinvocations.InsertForTesting(rootInv2)
			ms = append(ms, InsertForTesting(wu2Root)...)
			ms = append(ms, InsertForTesting(wu21)...)
			ms = append(ms, InsertForTesting(wu22)...)
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
				state, err := ReadState(span.Single(ctx), id)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, state, should.Equal(testData.State))
			})

			t.Run("not found", func(t *ftt.Test) {
				nonExistentID := ID{
					RootInvocationID: rootInvID,
					WorkUnitID:       "non-existent-id",
				}
				_, err := ReadState(span.Single(ctx), nonExistentID)
				st, ok := appstatus.Get(err)
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, st.Code(), should.Equal(codes.NotFound))
				assert.Loosely(t, st.Message(), should.ContainSubstring(`"rootInvocations/root-inv-id/workUnits/non-existent-id" not found`))
			})

			t.Run("empty root invocation ID", func(t *ftt.Test) {
				_, err := ReadState(span.Single(ctx), ID{WorkUnitID: "work-unit-id"})
				assert.That(t, err, should.ErrLike("rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				_, err := ReadState(span.Single(ctx), ID{RootInvocationID: rootInvID})
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
				assert.That(t, realms, should.Match([]string{
					"testproject:root2",
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
			wu2 := NewBuilder(rootInvID, "work-unit-id2").WithState(pb.WorkUnit_ACTIVE).Build()
			ms := InsertForTesting(wu2)

			// Create a further root invocation with a work unit.
			rootInv2 := rootinvocations.NewBuilder("root-inv-id2").Build()
			wuRoot := NewBuilder("root-inv-id2", "root").WithState(pb.WorkUnit_ACTIVE).Build()
			wu21 := NewBuilder("root-inv-id2", "work-unit-id").WithState(pb.WorkUnit_FINALIZING).Build()
			wu22 := NewBuilder("root-inv-id2", "work-unit-id2").WithState(pb.WorkUnit_FINALIZED).Build()
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
				states, err := ReadStates(span.Single(ctx), ids)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, states, should.Match([]pb.WorkUnit_State{
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
				_, err := ReadStates(span.Single(ctx), ids)
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
				_, err := ReadStates(span.Single(ctx), ids)
				assert.That(t, err, should.ErrLike("ids[1]: rootInvocationID: unspecified"))
			})

			t.Run("empty work unit ID", func(t *ftt.Test) {
				ids := []ID{
					id,
					{RootInvocationID: rootInvID},
				}
				_, err := ReadStates(span.Single(ctx), ids)
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

		t.Run("ReadChildren(Batch)", func(t *ftt.Test) {
			// Setup data for ReadChildren and ReadChildrenBatch tests.
			rootInvID := rootinvocations.ID("children-root")
			rootInv := rootinvocations.NewBuilder(rootInvID).Build()
			rootID := ID{RootInvocationID: rootInvID, WorkUnitID: "root"}
			root := NewBuilder(rootInvID, "root").Build()

			// parent1 has two children.
			parent1ID := ID{RootInvocationID: rootInvID, WorkUnitID: "parent1"}
			parent1 := NewBuilder(rootInvID, "parent1").Build()
			child11ID := ID{RootInvocationID: rootInvID, WorkUnitID: "child11"}
			child11 := NewBuilder(rootInvID, "child11").WithParentWorkUnitID("parent1").Build()
			child12ID := ID{RootInvocationID: rootInvID, WorkUnitID: "child12"}
			child12 := NewBuilder(rootInvID, "child12").WithParentWorkUnitID("parent1").Build()

			// parent2 has one child.
			parent2ID := ID{RootInvocationID: rootInvID, WorkUnitID: "parent2"}
			parent2 := NewBuilder(rootInvID, "parent2").Build()
			child21ID := ID{RootInvocationID: rootInvID, WorkUnitID: "child21"}
			child21 := NewBuilder(rootInvID, "child21").WithParentWorkUnitID("parent2").Build()

			// parent3 has no children.
			parent3ID := ID{RootInvocationID: rootInvID, WorkUnitID: "parent3"}
			parent3 := NewBuilder(rootInvID, "parent3").Build()

			ms := rootinvocations.InsertForTesting(rootInv)
			ms = append(ms, InsertForTesting(root)...)
			ms = append(ms, InsertForTesting(parent1)...)
			ms = append(ms, InsertForTesting(child11)...)
			ms = append(ms, InsertForTesting(child12)...)
			ms = append(ms, InsertForTesting(parent2)...)
			ms = append(ms, InsertForTesting(child21)...)
			ms = append(ms, InsertForTesting(parent3)...)
			testutil.MustApply(ctx, t, ms...)

			t.Run("ReadChildren", func(t *ftt.Test) {
				children, err := ReadChildren(span.Single(ctx), parent1ID)
				assert.Loosely(t, err, should.BeNil)

				assert.That(t, children, should.Match([]ID{child11ID, child12ID}))
			})

			t.Run("ReadChildrenBatch", func(t *ftt.Test) {
				t.Run("happy path", func(t *ftt.Test) {
					ids := []ID{
						rootID,
						parent1ID,
						parent2ID,
						parent3ID,
						{RootInvocationID: rootInvID, WorkUnitID: "non-existent-parent"},
						parent1ID, // Duplicate
					}

					results, err := ReadChildrenBatch(span.Single(ctx), ids)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, results, should.HaveLength(6))
					assert.That(t, results[0], should.Match([]ID{parent1ID, parent2ID, parent3ID}))
					assert.That(t, results[1], should.Match([]ID{child11ID, child12ID}))
					assert.That(t, results[2], should.Match([]ID{child21ID}))
					assert.That(t, results[3], should.Match([]ID{}))
					assert.That(t, results[4], should.Match([]ID{}))
					assert.That(t, results[5], should.Match([]ID{child11ID, child12ID}))

					// Check that slices for duplicate requests are not aliased.
					if len(results[4]) > 0 {
						results[4][0].WorkUnitID = "mutated"
						assert.That(t, results[0][0].WorkUnitID, should.Equal("child11"))
					}
				})

				t.Run("empty request", func(t *ftt.Test) {
					results, err := ReadChildrenBatch(span.Single(ctx), []ID{})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, results, should.HaveLength(0))
				})

				t.Run("different root invocations", func(t *ftt.Test) {
					otherParentID := ID{RootInvocationID: rootinvocations.ID("other-root"), WorkUnitID: "other-parent"}
					ids := []ID{parent1ID, otherParentID}
					_, err := ReadChildrenBatch(span.Single(ctx), ids)
					assert.That(t, err, should.ErrLike("all work units must belong to the same root invocation"))
				})
			})
		})
	})
}
