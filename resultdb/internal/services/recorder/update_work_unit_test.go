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
	"context"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
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

func TestValidateUpdateWorkUnitRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("TestValidateUpdateWorkUnitRequest", t, func(t *ftt.Test) {
		ctx := context.Background()
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		req := &pb.UpdateWorkUnitRequest{
			WorkUnit: &pb.WorkUnit{
				Name: "invocations/inv/workUnits/wu",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{}},
		}

		t.Run("empty update mask", func(t *ftt.Test) {
			err := validateUpdateWorkUnitRequest(ctx, req, cfg)
			assert.Loosely(t, err, should.ErrLike("update_mask: paths is empty"))
		})

		t.Run("non-exist update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"not_exist"}
			err := validateUpdateWorkUnitRequest(ctx, req, cfg)
			assert.Loosely(t, err, should.ErrLike(`update_mask: field "not_exist" does not exist in message WorkUnit`))
		})

		t.Run("unsupported update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"name"}
			err := validateUpdateWorkUnitRequest(ctx, req, cfg)
			assert.Loosely(t, err, should.ErrLike(`update_mask: unsupported path "name"`))
		})

		t.Run("submask in update mask", func(t *ftt.Test) {
			t.Run("unsupported", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"deadline.seconds"}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`update_mask: "deadline" should not have any submask`))
			})

			t.Run("supported for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.some_key"}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid key for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.invalid_key_"}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties: key "invalid_key_": does not match`))
			})

			t.Run("too deep for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.some_key.fields"}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties["some_key"] should not have any submask`))
			})
		})

		t.Run("state", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"state"}

			t.Run("valid FINALIZING", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_FINALIZING
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("valid ACTIVE", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_ACTIVE
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid FINALIZED", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_FINALIZED
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike("work_unit: state: must be FINALIZING or ACTIVE"))
			})
		})

		t.Run("deadline", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"deadline"}
			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid empty", func(t *ftt.Test) {
				req.WorkUnit.Deadline = nil
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`invalid nil Timestamp`))
			})

			t.Run("invalid past", func(t *ftt.Test) {
				req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: deadline: must be at least 10 seconds in the future`))
			})
		})

		t.Run("module_id", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"module_id"}

			t.Run("set for the first time", func(t *ftt.Test) {
				req.WorkUnit.ModuleId = nil
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
					ModuleName:    "module",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("structurally invalid", func(t *ftt.Test) {
				req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
					ModuleName:        "mymodule",
					ModuleScheme:      "gtest",
					ModuleVariantHash: "aaaaaaaaaaaaaaaa", // Variant hash only is not allowed for storage.
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike("work_unit: module_id: module_variant: unspecified"))
			})

			t.Run("invalid against config", func(t *ftt.Test) {
				req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
					ModuleName:    "mymodule",
					ModuleScheme:  "unknown", // Not defined in placeholder config.
					ModuleVariant: pbutil.Variant("k", "v"),
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: module_id: module_scheme: scheme "unknown" is not a known scheme`))
			})
		})

		t.Run("tags", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"tags"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "k", Value: "a\n"}}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: tags: "k":"a\n": value: non-printable rune '\n' at byte index 1`))
			})
		})

		t.Run("properties", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"properties"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"key_1": structpb.NewStringValue("value_1"),
					},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.Properties = &structpb.Struct{Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("1"),
				}}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: properties: must have a field "@type"`))
			})
		})

		t.Run("extended_properties", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"extended_properties"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.ExtendedProperties = map[string]*structpb.Struct{
					"key": {Fields: map[string]*structpb.Value{
						"a": structpb.NewStringValue("1"),
					}},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: extended_properties: ["key"]: must have a field "@type"`))
			})
		})

		t.Run("instructions", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"instructions"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Instructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id:              "step-1",
							Type:            pb.InstructionType_STEP_INSTRUCTION,
							DescriptiveName: "des_name",
						},
					},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid duplicate id", func(t *ftt.Test) {
				req.WorkUnit.Instructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{Id: "dup-id", Type: pb.InstructionType_STEP_INSTRUCTION, DescriptiveName: "des_name"},
						{Id: "dup-id", Type: pb.InstructionType_STEP_INSTRUCTION, DescriptiveName: "des_name"},
					},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, cfg)
				assert.Loosely(t, err, should.ErrLike(`work_unit: instructions: instructions[1]: id: "dup-id" is re-used at index 0`))
			})
		})
	})
}

func TestUpdateWorkUnit(t *testing.T) {
	ftt.Run("TestUpdateWorkUnit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)
		ctx, sched := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		recorder := newTestRecorderServer()
		// A basic valid request.
		rootInvID := rootinvocations.ID("rootid")
		wuID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu",
		}
		req := &pb.UpdateWorkUnitRequest{
			WorkUnit: &pb.WorkUnit{
				Name:  wuID.Name(),
				State: pb.WorkUnit_ACTIVE,
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
		}

		// Attach a valid update token.
		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		// Insert root invocation and work unit into spanner.
		expectedWURow := workunits.
			NewBuilder(wuID.RootInvocationID, wuID.WorkUnitID).
			WithModuleID(nil).
			WithState(pb.WorkUnit_ACTIVE).
			Build()

		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(expectedWURow)...)
		testutil.MustApply(ctx, t, ms...)

		expectedWU := masking.WorkUnit(expectedWURow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)

		t.Run("request validate", func(t *ftt.Test) {
			t.Run("unspecified work unit", func(t *ftt.Test) {
				req.WorkUnit = nil
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: work_unit: unspecified"))
			})

			t.Run("invalid name", func(t *ftt.Test) {
				req.WorkUnit.Name = "invalid"
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: work_unit: name: does not match pattern"))
			})

			t.Run("other invalid", func(t *ftt.Test) {
				// validateUpdateWorkUnitRequest has its own exhaustive test cases,
				// simply check that it is called.
				req.WorkUnit.State = pb.WorkUnit_FINALIZED
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: work_unit: state: must be FINALIZING or ACTIVE"))
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})

			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})
		})

		t.Run("etag", func(t *ftt.Test) {
			t.Run("bad etag", func(t *ftt.Test) {
				req.WorkUnit.Etag = "invalid"

				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike(`work_unit: etag: malformated etag`))
			})

			t.Run("unmatch etag", func(t *ftt.Test) {
				// Work unit updated.
				req.UpdateMask.Paths = []string{"tags"}
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "nk", Value: "nv"}}
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Request sent with the old etag.
				req.WorkUnit.Etag = expectedWU.Etag
				_, err = recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Aborted))
				assert.That(t, err, should.ErrLike(`the work unit was modified since it was last read; the update was not applied`))
			})

			t.Run("match etag", func(t *ftt.Test) {
				req.WorkUnit.Etag = masking.WorkUnitETag(expectedWURow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)

				_, err = recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("no work unit", func(t *ftt.Test) {
			nonexistWuID := workunits.ID{
				RootInvocationID: wuID.RootInvocationID,
				WorkUnitID:       "nonexist",
			}
			req.WorkUnit.Name = nonexistWuID.Name()
			token, err := generateWorkUnitUpdateToken(ctx, nonexistWuID)
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			_, err = recorder.UpdateWorkUnit(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`"rootInvocations/rootid/workUnits/nonexist" not found`))
		})

		t.Run("work unit not active", func(t *ftt.Test) {
			// Finalize work unit first.
			req.UpdateMask.Paths = []string{"state"}
			req.WorkUnit.State = pb.WorkUnit_FINALIZING

			_, err := recorder.UpdateWorkUnit(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			_, err = recorder.UpdateWorkUnit(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike(`"rootInvocations/rootid/workUnits/wu" is not active`))
		})

		t.Run("e2e", func(t *ftt.Test) {
			t.Run("base case - no update", func(t *ftt.Test) {
				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, wu, should.Match(expectedWU))
			})

			t.Run("state", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"state"}
				req.WorkUnit.State = pb.WorkUnit_FINALIZING

				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wu.FinalizeStartTime, should.NotBeNil)
				expectedWU.State = pb.WorkUnit_FINALIZING
				expectedWU.FinalizeStartTime = wu.FinalizeStartTime
				expectedWU.LastUpdated = wu.LastUpdated
				assert.That(t, wu, should.Match(expectedWU))

				// Validate work unit table.
				wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectedWURow.State = pb.WorkUnit_FINALIZING
				expectedWURow.FinalizeStartTime = wuRow.FinalizeStartTime
				expectedWURow.LastUpdated = wuRow.LastUpdated
				assert.Loosely(t, wuRow, should.Match(expectedWURow))
				assert.Loosely(t, wuRow.FinalizeStartTime.Valid, should.BeTrue)

				// Validate legacy invocation table.
				inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.State, should.Equal(pb.Invocation_FINALIZING))
				assert.Loosely(t, inv.FinalizeStartTime.CheckValid(), should.BeNil)

				// Enqueued the finalization task.
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match([]protoreflect.ProtoMessage{
					&taskspb.RunExportNotifications{InvocationId: string(wuID.LegacyInvocationID())},
					&taskspb.TryFinalizeInvocation{InvocationId: string(wuID.LegacyInvocationID())},
				}))
			})

			t.Run("module_id", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"module_id"}
				newModuleID := &pb.ModuleIdentifier{
					ModuleName:    "module",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				}
				req.WorkUnit.ModuleId = newModuleID

				t.Run("set for the first time", func(t *ftt.Test) {
					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					expectedWU.ModuleId = newModuleID
					expectedWU.LastUpdated = wu.LastUpdated
					pbutil.PopulateModuleIdentifierHashes(expectedWU.ModuleId)
					assert.That(t, wu, should.Match(expectedWU))

					// Validate work unit table.
					wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					expectedWURow.ModuleID = newModuleID
					expectedWURow.LastUpdated = wuRow.LastUpdated
					assert.Loosely(t, wuRow, should.Match(expectedWURow))

					// Validate legacy invocation table.
					inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.ModuleId, should.Match(newModuleID))
				})
				t.Run("updating an already set module", func(t *ftt.Test) {
					// Set a module ID first.
					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, wu.ModuleId.ModuleName, should.Equal("module"))

					t.Run("to nil", func(t *ftt.Test) {
						req.WorkUnit.ModuleId = nil
						_, err = recorder.UpdateWorkUnit(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`work_unit: module_id: cannot modify module_id once set (do you need to create a child work unit?); got nil, was non-nil`))
					})
					t.Run("to another value", func(t *ftt.Test) {
						req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
							ModuleName:    "new_module",
							ModuleScheme:  "gtest",
							ModuleVariant: pbutil.Variant("k", "v"),
						}
						_, err = recorder.UpdateWorkUnit(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`work_unit: module_id: cannot modify module_id once set`))
					})
					t.Run("to the same value", func(t *ftt.Test) {
						// This is allowed, as it is a no-op.
						_, err = recorder.UpdateWorkUnit(ctx, req)
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

				updateExtendedProperties := func(extendedPropertiesOrg map[string]*structpb.Struct) {
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
					extendedPropertiesOrg := map[string]*structpb.Struct{
						"old_key": structValueOrg,
					}
					extendedPropertiesNew := map[string]*structpb.Struct{
						"new_key": structValueOrg,
					}
					updateMask := &field_mask.FieldMask{Paths: []string{"extended_properties"}}
					updateExtendedProperties(extendedPropertiesOrg)
					req.WorkUnit.ExtendedProperties = extendedPropertiesNew
					req.UpdateMask = updateMask

					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					expectedWU.LastUpdated = wu.LastUpdated
					assert.Loosely(t, wu.ExtendedProperties, should.Match(extendedPropertiesNew))

					// Validate work unit table.
					wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					expectedWURow.LastUpdated = wuRow.LastUpdated
					expectedWURow.ExtendedProperties = extendedPropertiesNew
					assert.Loosely(t, wuRow, should.Match(expectedWURow))

					// Validate legacy invocation table.
					inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.ExtendedProperties, should.Match(extendedPropertiesNew))
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
					updateMask := &field_mask.FieldMask{Paths: []string{
						"extended_properties.to_be_added",
						"extended_properties.to_be_replaced",
						"extended_properties.to_be_deleted",
					}}
					expectedExtendedProperties := map[string]*structpb.Struct{
						"to_be_kept":     structValueOrg,
						"to_be_added":    structValueNew,
						"to_be_replaced": structValueNew,
					}
					updateExtendedProperties(extendedPropertiesOrg)
					req.WorkUnit.ExtendedProperties = extendedPropertiesNew
					req.UpdateMask = updateMask

					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					expectedWU.LastUpdated = wu.LastUpdated
					assert.Loosely(t, wu.ExtendedProperties, should.Match(expectedExtendedProperties))

					// Validate work unit table.
					wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					expectedWURow.LastUpdated = wuRow.LastUpdated
					expectedWURow.ExtendedProperties = expectedExtendedProperties
					assert.Loosely(t, wuRow, should.Match(expectedWURow))

					// Validate legacy invocation table.
					inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.ExtendedProperties, should.Match(expectedExtendedProperties))
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
					updateExtendedProperties(extendedPropertiesOrg)
					req.WorkUnit.ExtendedProperties = extendedPropertiesNew
					req.UpdateMask = updateMask

					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`work_unit: extended_properties: exceeds the maximum size of`))
					assert.Loosely(t, wu, should.BeNil)
				})
			})

			t.Run("updated time", func(t *ftt.Test) {
				t.Run("update when there is a update", func(t *ftt.Test) {
					req.UpdateMask.Paths = []string{"deadline"}
					req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
					oldUpdateTime := expectedWURow.LastUpdated

					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wu.LastUpdated.AsTime(), should.HappenAfter(oldUpdateTime))

					// Check the work unit table.
					wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wuRow.LastUpdated, should.HappenAfter(oldUpdateTime))
				})

				t.Run("not updated when there is a no-op", func(t *ftt.Test) {
					req.UpdateMask.Paths = []string{"state"}
					req.WorkUnit.State = pb.WorkUnit_ACTIVE
					oldUpdateTime := expectedWURow.LastUpdated

					wu, err := recorder.UpdateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wu.LastUpdated.AsTime(), should.Match(oldUpdateTime))

					// Check the work unit table.
					wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, wuRow.LastUpdated, should.Match(oldUpdateTime))
				})
			})

			t.Run("deadline, properties, instructions, tags", func(t *ftt.Test) {
				newDeadline := pbutil.MustTimestampProto(now.Add(3 * time.Hour))
				instruction := testutil.TestInstructions()
				updateMask := &field_mask.FieldMask{
					Paths: []string{"deadline", "properties", "instructions", "tags"},
				}
				newProperties := testutil.TestStrictProperties()
				req := &pb.UpdateWorkUnitRequest{
					WorkUnit: &pb.WorkUnit{
						Name:         wuID.Name(),
						Deadline:     newDeadline,
						Properties:   newProperties,
						Instructions: instruction,
						Tags:         []*pb.StringPair{{Key: "newkey", Value: "newvalue"}},
						State:        pb.WorkUnit_FINALIZING,
					},
					UpdateMask: updateMask,
				}
				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedWU.Deadline = newDeadline
				expectedWU.Properties = newProperties
				expectedWU.Instructions = instructionutil.InstructionsWithNames(instruction, wuID.Name())
				expectedWU.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
				expectedWU.LastUpdated = wu.LastUpdated
				assert.That(t, wu, should.Match(expectedWU))

				// Validate work unit table.
				wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectedWURow.Deadline = newDeadline.AsTime()
				expectedWURow.Properties = newProperties
				expectedWURow.Instructions = instructionutil.InstructionsWithNames(instruction, wuID.Name())
				expectedWURow.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
				expectedWURow.LastUpdated = wuRow.LastUpdated
				assert.Loosely(t, wuRow, should.Match(expectedWURow))

				// Validate legacy invocation table.
				inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.Deadline, should.Match(newDeadline))
				assert.Loosely(t, inv.Properties, should.Match(newProperties))
				assert.Loosely(t, inv.Instructions, should.Match(instructionutil.InstructionsWithNames(instruction, wuID.LegacyInvocationID().Name())))
				assert.Loosely(t, inv.Tags, should.Match([]*pb.StringPair{{Key: "newkey", Value: "newvalue"}}))
			})
		})
	})
}
