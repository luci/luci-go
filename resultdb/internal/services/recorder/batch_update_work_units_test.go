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

package recorder

import (
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestBatchUpdateWorkUnits(t *testing.T) {
	ftt.Run("TestBatchUpdateWorkUnits", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)
		ctx, sched := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		recorder := newTestRecorderServer()
		rootInvID := rootinvocations.ID("rootid")
		rootWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: workunits.RootWorkUnitID}

		wuID1 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root:wu1"}
		wuID2 := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root:wu2"}

		// Attach a valid update token for the root invocation.
		token, err := generateWorkUnitUpdateToken(ctx, rootWuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		// Insert root invocation and work units into spanner.
		wuRow1Expected := workunits.
			NewBuilder(wuID1.RootInvocationID, wuID1.WorkUnitID).
			WithState(pb.WorkUnit_ACTIVE).
			WithModuleID(nil).
			Build()
		wuRow2Expected := workunits.
			NewBuilder(wuID2.RootInvocationID, wuID2.WorkUnitID).
			WithState(pb.WorkUnit_ACTIVE).
			WithTags([]*pb.StringPair{{Key: "k2", Value: "v2"}}).
			WithModuleID(nil).
			Build()

		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(wuRow1Expected)...)
		ms = append(ms, insert.WorkUnit(wuRow2Expected)...)
		testutil.MustApply(ctx, t, ms...)

		wu1Expected := masking.WorkUnit(wuRow1Expected, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
		wu2Expected := masking.WorkUnit(wuRow2Expected, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)

		// A basic valid no-op request for two work units.
		req := &pb.BatchUpdateWorkUnitsRequest{
			Requests: []*pb.UpdateWorkUnitRequest{
				{
					WorkUnit: &pb.WorkUnit{
						Name:  wuID1.Name(),
						State: pb.WorkUnit_ACTIVE,
					},
					UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
				},
				{
					WorkUnit: &pb.WorkUnit{
						Name:  wuID2.Name(),
						State: pb.WorkUnit_ACTIVE,
					},
					UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
				},
			},
		}

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("empty request", func(t *ftt.Test) {
				req.Requests = []*pb.UpdateWorkUnitRequest{}

				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
			})

			t.Run("sub-request", func(t *ftt.Test) {
				t.Run("work unit", func(t *ftt.Test) {
					t.Run("unspecified work unit", func(t *ftt.Test) {
						req.Requests[1].WorkUnit = nil

						_, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("bad request: requests[1]: work_unit: unspecified"))
					})

					t.Run("invalid name", func(t *ftt.Test) {
						req.Requests[0].WorkUnit.Name = "invalid"

						_, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("bad request: requests[0]: work_unit: name: does not match pattern"))
					})

					t.Run("invalid state", func(t *ftt.Test) {
						// validateUpdateWorkUnitRequest has its own exhaustive test cases,
						// simply check that it is called for each sub-request.
						req.Requests[1].WorkUnit.State = pb.WorkUnit_FINALIZED
						req.Requests[1].UpdateMask.Paths = []string{"state"}

						_, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("bad request: requests[1]: work_unit: state: must be FINALIZING or ACTIVE"))
					})

					t.Run("contain duplicated work unit", func(t *ftt.Test) {
						req.Requests[1].WorkUnit.Name = req.Requests[0].WorkUnit.Name

						_, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: name: duplicated work unit with requests[0]"))
					})
				})
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})

			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})

			t.Run("different root invocation", func(t *ftt.Test) {
				otherRootInvID := rootinvocations.ID("otherroot")
				otherWuID := workunits.ID{RootInvocationID: otherRootInvID, WorkUnitID: "otherwu"}
				req.Requests[1].WorkUnit.Name = otherWuID.Name()

				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: name: all requests must be for the same root invocation"))
			})

			t.Run("require different update tokens", func(t *ftt.Test) {
				otherWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "otherwu"}
				req.Requests[1].WorkUnit.Name = otherWuID.Name()

				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit: name "rootInvocations/rootid/workUnits/otherwu" requires a different update token to requests[0]'s "work_unit: name" "rootInvocations/rootid/workUnits/root:wu1"`))
			})
		})

		t.Run("etag", func(t *ftt.Test) {
			t.Run("bad etag", func(t *ftt.Test) {
				req.Requests[1].WorkUnit.Etag = "invalid"

				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`requests[1]: work_unit: etag: malformated etag`))
			})

			t.Run("etag does not match", func(t *ftt.Test) {
				// Work unit updated to change its etag.
				req.Requests[1].WorkUnit.Tags = []*pb.StringPair{{Key: "updatedkey", Value: "updatedval"}}
				req.Requests[1].UpdateMask.Paths = []string{"tags"}
				_, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Re-sent with the old etag.
				req.Requests[1].WorkUnit.Etag = wu1Expected.Etag
				_, err = recorder.BatchUpdateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Aborted))
				assert.That(t, err, should.ErrLike(`requests[1]: the work unit was modified since it was last read; the update was not applied`))
			})
		})
		t.Run("work unit not found", func(t *ftt.Test) {
			nonexistWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root:nonexist"}
			req.Requests[1].WorkUnit.Name = nonexistWuID.Name()

			_, err := recorder.BatchUpdateWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/rootid/workUnits/root:nonexist" not found`))
		})

		t.Run("work unit not active", func(t *ftt.Test) {
			// Finalize the second work unit.
			finalizeReq := &pb.UpdateWorkUnitRequest{
				WorkUnit:   &pb.WorkUnit{Name: wuID2.Name(), State: pb.WorkUnit_FINALIZING},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			}
			req.Requests[1] = finalizeReq
			_, err = recorder.BatchUpdateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Now try to update both. The transaction should fail.
			req.Requests[0].UpdateMask.Paths = []string{"tags"}
			req.Requests[1].UpdateMask.Paths = []string{"tags"}
			_, err = recorder.BatchUpdateWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.That(t, err, should.ErrLike(`requests[1]: work unit "rootInvocations/rootid/workUnits/root:wu2" is not active`))
		})

		t.Run("e2e", func(t *ftt.Test) {
			assertWUProtoUpdated := func(wu *pb.WorkUnit, expected *pb.WorkUnit) {
				// Etag is must be different if updated.
				assert.That(t, wu.Etag, should.NotEqual(expected.Etag), truth.LineContext())
				assert.Loosely(t, wu.Etag, should.NotBeEmpty, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, wu.LastUpdated.AsTime(), should.HappenAfter(expected.LastUpdated.AsTime()), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				if expected.State != pb.WorkUnit_ACTIVE {
					assert.Loosely(t, wu.FinalizeStartTime, should.Match(wu.LastUpdated), truth.LineContext())
				} else {
					assert.Loosely(t, wu.FinalizeStartTime, should.BeNil, truth.LineContext())
				}
				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expected.LastUpdated = wu.LastUpdated
				expected.Etag = wu.Etag
				expected.FinalizeStartTime = wu.FinalizeStartTime
				assert.That(t, wu, should.Match(expected), truth.LineContext())
			}

			assertWUSpannerUpdated := func(wuID workunits.ID, expectedRow *workunits.WorkUnitRow) {
				wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, wuRow.LastUpdated, should.HappenAfter(expectedRow.LastUpdated), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				shouldSetfinalizeStartTime := expectedRow.State != pb.WorkUnit_ACTIVE
				assert.Loosely(t, wuRow.FinalizeStartTime.Valid, should.Equal(shouldSetfinalizeStartTime), truth.LineContext())
				if shouldSetfinalizeStartTime {
					assert.Loosely(t, wuRow.FinalizeStartTime.Time, should.Match(wuRow.LastUpdated), truth.LineContext())
				}

				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expectedRow.LastUpdated = wuRow.LastUpdated
				expectedRow.FinalizeStartTime = wuRow.FinalizeStartTime
				assert.That(t, wuRow, should.Match(expectedRow), truth.LineContext())
			}

			t.Run("base case - no update", func(t *ftt.Test) {
				res, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.WorkUnits, should.HaveLength(2))
				assert.That(t, res.WorkUnits[0], should.Match(wu1Expected))
				assert.That(t, res.WorkUnits[1], should.Match(wu2Expected))

				// Check the work unit table.
				wuRow, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, wuRow, should.Match(wuRow1Expected))
				wuRow, err = workunits.Read(span.Single(ctx), wuID2, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, wuRow, should.Match(wuRow2Expected))
			})

			t.Run("update one, no-op for another", func(t *ftt.Test) {
				t.Run("state", func(t *ftt.Test) {
					req.Requests[1].UpdateMask.Paths = []string{"state"}
					req.Requests[1].WorkUnit.State = pb.WorkUnit_FINALIZING

					res, err := recorder.BatchUpdateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.WorkUnits, should.HaveLength(2))

					wu2Expected.State = pb.WorkUnit_FINALIZING
					assertWUProtoUpdated(res.WorkUnits[1], wu2Expected)

					// Validate work unit table.
					wuRow2Expected.State = pb.WorkUnit_FINALIZING
					assertWUSpannerUpdated(wuID2, wuRow2Expected)

					// Validate legacy invocation table.
					inv, err := invocations.Read(span.Single(ctx), wuID2.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
					assert.Loosely(t, inv.FinalizeStartTime.CheckValid(), should.BeNil)

					// Enqueued the finalization task.
					assert.Loosely(t, sched.Tasks().Payloads(), should.Match([]protoreflect.ProtoMessage{
						&taskspb.RunExportNotifications{InvocationId: string(wuID2.LegacyInvocationID())},
						&taskspb.TryFinalizeInvocation{InvocationId: string(wuID2.LegacyInvocationID())},
					}))

					// Work unit 1 is no-op.
					assert.That(t, res.WorkUnits[0], should.Match(wu1Expected))
					wuRow, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wuRow, should.Match(wuRow1Expected))
				})

				t.Run("module_id", func(t *ftt.Test) {
					newModuleID := &pb.ModuleIdentifier{
						ModuleName:    "module",
						ModuleScheme:  "gtest",
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					req.Requests[1].UpdateMask.Paths = []string{"module_id"}
					req.Requests[1].WorkUnit.ModuleId = newModuleID

					t.Run("set for the first time", func(t *ftt.Test) {
						res, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						wu2Expected.ModuleId = newModuleID
						pbutil.PopulateModuleIdentifierHashes(wu2Expected.ModuleId)
						assertWUProtoUpdated(res.WorkUnits[1], wu2Expected)

						// Validate work unit table.
						wuRow2Expected.ModuleID = newModuleID
						assertWUSpannerUpdated(wuID2, wuRow2Expected)

						// Validate legacy invocation table.
						inv, err := invocations.Read(span.Single(ctx), wuID2.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, inv.ModuleId, should.Match(newModuleID))

						// Work unit 1 is no-op.
						assert.That(t, res.WorkUnits[0], should.Match(wu1Expected))
						wuRow, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, wuRow, should.Match(wuRow1Expected))
					})
					t.Run("updating an already set module", func(t *ftt.Test) {
						// Set a module ID first.
						_, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)

						t.Run("to nil", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.ModuleId = nil
							_, err = recorder.BatchUpdateWorkUnits(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.That(t, err, should.ErrLike(`requests[1]: work_unit: module_id: cannot modify module_id once set (do you need to create a child work unit?); got nil, was non-nil`))
						})
						t.Run("to another value", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.ModuleId = &pb.ModuleIdentifier{
								ModuleName:    "new_module",
								ModuleScheme:  "gtest",
								ModuleVariant: pbutil.Variant("k", "v"),
							}
							_, err = recorder.BatchUpdateWorkUnits(ctx, req)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.That(t, err, should.ErrLike(`requests[1]: work_unit: module_id: cannot modify module_id once set`))
						})
						t.Run("to the same value", func(t *ftt.Test) {
							// This is allowed, as it is a no-op.
							_, err = recorder.BatchUpdateWorkUnits(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
					})
				})

				t.Run("extended_properties", func(t *ftt.Test) {
					structValueOrg := &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"child_key_1": structpb.NewStringValue("child_value_1"),
						},
					}
					structValueNew := &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"child_key_2": structpb.NewStringValue("child_value_2"),
						},
					}

					updateExtendedProperties := func(wuID workunits.ID, extendedPropertiesOrg map[string]*structpb.Struct) {
						internalExtendedProperties := &invocationspb.ExtendedProperties{
							ExtendedProperties: extendedPropertiesOrg,
						}
						testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
							"RootInvocationShardId": wuID.RootInvocationShardID(),
							"WorkUnitId":            wuID.WorkUnitID,
							"ExtendedProperties":    spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
						}))
					}

					t.Run("replace entire field", func(t *ftt.Test) {
						extendedPropertiesOrg := map[string]*structpb.Struct{"old_key": structValueOrg}
						extendedPropertiesNew := map[string]*structpb.Struct{"new_key": structValueOrg}
						updateExtendedProperties(wuID1, extendedPropertiesOrg)
						req.Requests[0].WorkUnit.ExtendedProperties = extendedPropertiesNew
						req.Requests[0].UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{"extended_properties"}}

						res, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						// Extended properties field is elided from the response.
						assertWUProtoUpdated(res.WorkUnits[0], wu1Expected)

						// Validate work unit table.
						wuRow1Expected.ExtendedProperties = extendedPropertiesNew
						assertWUSpannerUpdated(wuID1, wuRow1Expected)

						// Validate legacy invocation table.
						inv, err := invocations.Read(span.Single(ctx), wuID1.LegacyInvocationID(), invocations.AllFields)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, inv.ExtendedProperties, should.Match(extendedPropertiesNew))

						// Work unit 2 is no-op.
						assert.That(t, res.WorkUnits[1], should.Match(wu2Expected))
						wuRow, err := workunits.Read(span.Single(ctx), wuID2, workunits.AllFields)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, wuRow, should.Match(wuRow2Expected))
					})
					t.Run("add, replace, and delete keys to existing field", func(t *ftt.Test) {
						extendedPropertiesOrg := map[string]*structpb.Struct{
							"to_be_kept":     structValueOrg,
							"to_be_replaced": structValueOrg,
							"to_be_deleted":  structValueOrg,
						}
						extendedPropertiesNew := map[string]*structpb.Struct{
							"to_be_added":    structValueNew,
							"to_be_replaced": structValueNew,
						}
						expectedExtendedProperties := map[string]*structpb.Struct{
							"to_be_kept":     structValueOrg,
							"to_be_added":    structValueNew,
							"to_be_replaced": structValueNew,
						}
						updateExtendedProperties(wuID1, extendedPropertiesOrg)
						req.Requests[0].WorkUnit.ExtendedProperties = extendedPropertiesNew
						req.Requests[0].UpdateMask = &fieldmaskpb.FieldMask{Paths: []string{
							"extended_properties.to_be_added",
							"extended_properties.to_be_replaced",
							"extended_properties.to_be_deleted",
						}}

						res, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						// Extended properties field is elided from the response.
						assertWUProtoUpdated(res.WorkUnits[0], wu1Expected)

						// Validate work unit table.
						wuRow1Expected.ExtendedProperties = expectedExtendedProperties
						assertWUSpannerUpdated(wuID1, wuRow1Expected)

						// Validate legacy invocation table.
						inv, err := invocations.Read(span.Single(ctx), wuID1.LegacyInvocationID(), invocations.AllFields)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, inv.ExtendedProperties, should.Match(expectedExtendedProperties))

						// Work unit 2 is no-op.
						assert.That(t, res.WorkUnits[1], should.Match(wu2Expected))
						wuRow, err := workunits.Read(span.Single(ctx), wuID2, workunits.AllFields)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, wuRow, should.Match(wuRow2Expected))
					})

					t.Run("valid request but overall size exceed limit", func(t *ftt.Test) {
						structValueLong := &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
								"child_key_1": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationExtendedPropertyValue-80)),
							},
						}
						extendedPropertiesOrg := map[string]*structpb.Struct{
							"mykey_1": structValueLong,
							"mykey_2": structValueLong,
							"mykey_3": structValueLong,
							"mykey_4": structValueLong,
							"mykey_5": structValueOrg,
						}
						extendedPropertiesNew := map[string]*structpb.Struct{
							"mykey_5": structValueLong,
						}
						updateMask := &field_mask.FieldMask{Paths: []string{
							"extended_properties.mykey_5",
						}}
						updateExtendedProperties(wuID1, extendedPropertiesOrg)
						req.Requests[0].WorkUnit.ExtendedProperties = extendedPropertiesNew
						req.Requests[0].UpdateMask = updateMask

						wu, err := recorder.BatchUpdateWorkUnits(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: work_unit: extended_properties: exceeds the maximum size of`))
						assert.Loosely(t, wu, should.BeNil)
					})
				})

				t.Run("deadline, properties, instructions, tags", func(t *ftt.Test) {
					newDeadline := pbutil.MustTimestampProto(now.Add(3 * time.Hour))
					newProperties := testutil.TestStrictProperties()
					newTags := []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
					instruction := testutil.TestInstructions()
					updateMask := &field_mask.FieldMask{
						Paths: []string{"deadline", "properties", "instructions", "tags"},
					}
					req.Requests[1] = &pb.UpdateWorkUnitRequest{
						WorkUnit: &pb.WorkUnit{
							Name:         wuID2.Name(),
							Deadline:     newDeadline,
							Properties:   newProperties,
							Instructions: instruction,
							Tags:         newTags,
							State:        pb.WorkUnit_FINALIZING,
						},
						UpdateMask: updateMask,
					}

					res, err := recorder.BatchUpdateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					wu2Expected.Deadline = newDeadline
					wu2Expected.Properties = newProperties
					wu2Expected.Instructions = instructionutil.InstructionsWithNames(instruction, wuID2.Name())
					wu2Expected.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
					assertWUProtoUpdated(res.WorkUnits[1], wu2Expected)

					// Validate spanner.
					wuRow2Expected.Deadline = newDeadline.AsTime()
					wuRow2Expected.Properties = newProperties
					wuRow2Expected.Instructions = instructionutil.InstructionsWithNames(instruction, wuID2.Name())
					wuRow2Expected.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
					assertWUSpannerUpdated(wuID2, wuRow2Expected)

					// Validate legacy invocation table.
					inv, err := invocations.Read(span.Single(ctx), wuID2.LegacyInvocationID(), invocations.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.Deadline, should.Match(newDeadline))
					assert.Loosely(t, inv.Properties, should.Match(newProperties))
					assert.Loosely(t, inv.Instructions, should.Match(instructionutil.InstructionsWithNames(instruction, wuID2.LegacyInvocationID().Name())))
					assert.Loosely(t, inv.Tags, should.Match([]*pb.StringPair{{Key: "newkey", Value: "newvalue"}}))

					// Work unit 1 is no-op.
					assert.That(t, res.WorkUnits[0], should.Match(wu1Expected))
					wuRow, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wuRow, should.Match(wuRow1Expected))
				})
			})
			t.Run("update both", func(t *ftt.Test) {
				// Update tags for wu1 and deadline for wu2.
				newWu1Tags := []*pb.StringPair{{Key: "k1_new", Value: "v1_new"}}
				req.Requests[0].WorkUnit.Tags = newWu1Tags
				req.Requests[0].UpdateMask.Paths = []string{"tags"}
				newWu2Deadline := pbutil.MustTimestampProto(now.Add(3 * time.Hour))
				req.Requests[1].WorkUnit.Deadline = newWu2Deadline
				req.Requests[1].UpdateMask.Paths = []string{"deadline"}

				res, err := recorder.BatchUpdateWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.WorkUnits, should.HaveLength(2))

				// Verify wu1 response
				wu1Expected.Tags = req.Requests[0].WorkUnit.Tags
				assertWUProtoUpdated(res.WorkUnits[0], wu1Expected)

				// Verify wu2 response
				wu2Expected.Deadline = newWu2Deadline
				assertWUProtoUpdated(res.WorkUnits[1], wu2Expected)

				// Verify Spanner state for wu1
				wuRow1Expected.Tags = newWu1Tags
				assertWUSpannerUpdated(wuID1, wuRow1Expected)

				// Verify Spanner state for wu2
				wuRow2Expected.Deadline = newWu2Deadline.AsTime()
				assertWUSpannerUpdated(wuID2, wuRow2Expected)

				// Verify legacy invocation for wu1 was also updated
				inv, err := invocations.Read(span.Single(ctx), wuID1.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.Tags, should.Match(newWu1Tags))

				// Verify legacy invocation for wu2 was also updated
				inv, err = invocations.Read(span.Single(ctx), wuID2.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.Deadline, should.Match(newWu2Deadline))

				// Verify finalization task was enqueued for wu2
				assert.Loosely(t, sched.Tasks().Payloads(), should.HaveLength(0))
			})
		})
	})
}
