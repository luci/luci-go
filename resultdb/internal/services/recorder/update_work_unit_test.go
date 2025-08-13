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
	"testing"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
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

			t.Run("nil", func(t *ftt.Test) {
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

		recorder := newTestRecorderServer()
		// A basic valid request.
		wuID := workunits.ID{
			RootInvocationID: rootinvocations.ID("rootid"),
			WorkUnitID:       "wu",
		}
		req := &pb.UpdateWorkUnitRequest{
			WorkUnit:   &pb.WorkUnit{Name: wuID.Name()},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
		}

		// Attach a valid update token.
		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))
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
		t.Run("happy path", func(t *ftt.Test) {
			// TODO
		})
	})
}
