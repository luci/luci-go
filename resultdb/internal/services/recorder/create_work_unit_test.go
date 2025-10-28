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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateCreateWorkUnitRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	ftt.Run("ValidateCreateWorkUnitRequest", t, func(t *ftt.Test) {
		// Construct a valid request.
		req := &pb.CreateWorkUnitRequest{
			Parent:     "rootInvocations/u-my-root-id/workUnits/root",
			WorkUnitId: "u-my-work-unit-id",
			WorkUnit: &pb.WorkUnit{
				Realm: "project:realm",
			},
			RequestId: "request-id",
		}

		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run("valid", func(t *ftt.Test) {
			err := validateCreateWorkUnitRequest(req, cfg)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("work_unit", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.WorkUnit = nil
				err := validateCreateWorkUnitRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("work_unit: unspecified"))
			})
			t.Run("realm", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.WorkUnit.Realm = ""
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: realm: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Realm = "invalid:"
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: realm: bad global realm name"))
				})
			})
			t.Run("state", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// If it is unset, we will populate a default value.
					req.WorkUnit.State = pb.WorkUnit_STATE_UNSPECIFIED
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_RUNNING
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid (mask)", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_FINAL_STATE_MASK
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: state: FINAL_STATE_MASK is not a valid state"))
				})
				t.Run("invalid (final)", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_SUCCEEDED
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: state: work unit may not be created in a final state (got SUCCEEDED)"))
				})
				t.Run("invalid (out of range)", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_State(999)
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: state: unknown state 999"))
				})
			})
			t.Run("deadline", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// Empty is valid, the deadline will be defaulted.
					req.WorkUnit.Deadline = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: deadline: must be at least 10 seconds in the future"))
				})
			})
			t.Run("module_id", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					// This is valid.
					req.WorkUnit.ModuleId = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "gtest", // This is in the service config we use for testing.
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("structurally invalid", func(t *ftt.Test) {
					// pbutil.ValidateModuleIdentifierForStorage has its own
					// exhaustive tests, verify it is being called.
					req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:        "mymodule",
						ModuleScheme:      "gtest",
						ModuleVariantHash: "aaaaaaaaaaaaaaaa", // Variant hash only is not allowed for storage.
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: module_id: module_variant: unspecified"))
				})
				t.Run("invalid with respect to service configuration", func(t *ftt.Test) {
					req.WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "cooltest", // This is not defined in the service config.
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`work_unit: module_id: module_scheme: scheme "cooltest" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
				})
			})
			t.Run("producer_resource", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = ""
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = "//cr-buildbucket.appspot.com/builds/1234567890"
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = "invalid"
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: producer_resource: resource name \"invalid\" does not start with '//'"))
				})
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Tags = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`work_unit: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.WorkUnit.Tags = tags
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Properties = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`work_unit: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("extended_properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.ExtendedProperties = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid key", func(t *ftt.Test) {
					req.WorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					req.WorkUnit.ExtendedProperties["invalid_key@"] = &structpb.Struct{}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`work_unit: extended_properties: key "invalid_key@"`))
				})
			})
			t.Run("instructions", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Instructions = nil
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.Instructions = &pb.Instructions{
						Instructions: []*pb.Instruction{
							{
								Id:              "step",
								Type:            pb.InstructionType_STEP_INSTRUCTION,
								DescriptiveName: "Step Instruction",
								Name:            "random1",
								TargetedInstructions: []*pb.TargetedInstruction{
									{
										Targets: []pb.InstructionTarget{
											pb.InstructionTarget_LOCAL,
											pb.InstructionTarget_REMOTE,
										},
										Content: "step instruction",
										Dependencies: []*pb.InstructionDependency{
											{
												InvocationId:  "dep_inv_id",
												InstructionId: "dep_ins_id",
											},
										},
									},
								},
							},
						},
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Instructions = &pb.Instructions{
						Instructions: []*pb.Instruction{
							{},
						},
					}
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("work_unit: instructions: instructions[0]: id: unspecified"))
				})
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// Not required (in the individual requests of a batch request).
					req.RequestId = ""
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("invalid", func(t *ftt.Test) {
					req.RequestId = "😃"
					err := validateCreateWorkUnitRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
				})
			})
		})
	})
}

func TestWorkUnitToken(t *testing.T) {
	t.Parallel()

	ftt.Run("WorkUnitToken", t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()
		t.Run("round-trip", func(t *ftt.Test) {
			t.Run("work unit id not prefixed", func(t *ftt.Test) {
				id := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				token, err := generateWorkUnitUpdateToken(ctx, id)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)

				err = validateWorkUnitUpdateToken(ctx, token, id)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("work unit ID prefixed", func(t *ftt.Test) {
				idWithPrefix := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "base:a2",
				}
				token, err := generateWorkUnitUpdateToken(ctx, idWithPrefix)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)

				idWithPrefix.WorkUnitID = "base:a1"
				err = validateWorkUnitUpdateToken(ctx, token, idWithPrefix)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("workUnitTokenState", func(t *ftt.Test) {
			t.Run("work unit id not prefixed", func(t *ftt.Test) {
				id := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				assert.Loosely(t, workUnitUpdateTokenState(id), should.Equal(`"root-inv-id";"work-unit-id"`))
			})
			t.Run("work unit ID prefixed", func(t *ftt.Test) {
				id := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "base:a2",
				}
				assert.Loosely(t, workUnitUpdateTokenState(id), should.Equal(`"root-inv-id";"base"`))
			})
		})
	})
}

func TestCreateWorkUnit(t *testing.T) {
	ftt.Run(`TestCreateWorkUnit`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID},
				{Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource},
			},
		}
		ctx = auth.WithState(ctx, authState)

		// Set test clock.
		start := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, start)

		// Setup a full HTTP server to retrieve response headers.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		// Insert a root invocation and the parent work unit.
		rootInvID := rootinvocations.ID("root-inv-id")
		parentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent",
		}
		prefixedParentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:parent",
		}
		workUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new",
		}
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
		parentWu := workunits.NewBuilder(rootInvID, parentWorkUnitID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		parentPrefixedWu := workunits.NewBuilder(rootInvID, prefixedParentWorkUnitID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentWu)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentPrefixedWu)...)

		// Generate an update token for the parent work unit.
		parentUpdateToken, err := generateWorkUnitUpdateToken(ctx, parentWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))

		// A basic valid request.
		req := &pb.CreateWorkUnitRequest{
			Parent:     parentWorkUnitID.Name(),
			WorkUnitId: workUnitID.WorkUnitID,
			WorkUnit: &pb.WorkUnit{
				Realm: "testproject:testrealm",
				State: pb.WorkUnit_RUNNING,
			},
			RequestId: "test-request-id",
		}

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.Parent = ""
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid"
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: parent: does not match"))
				})
			})
			t.Run("work_unit", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.WorkUnit = nil
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: work_unit: unspecified"))
				})
				t.Run("realm", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.WorkUnit.Realm = ""
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("bad request: work_unit: realm: unspecified"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.WorkUnit.Realm = "invalid:"
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("bad request: work_unit: realm: bad global realm name"))
					})
				})
				t.Run("other invalid", func(t *ftt.Test) {
					// validateCreateWorkUnitRequest has its own exhaustive test cases,
					// simply check that it is called.
					req.WorkUnit.State = pb.WorkUnit_FINAL_STATE_MASK
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: work_unit: state: FINAL_STATE_MASK is not a valid state"))
				})
			})
			t.Run("work_unit_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnitId = ""
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: work_unit_id: unspecified"))
				})
				t.Run("reserved", func(t *ftt.Test) {
					req.WorkUnitId = "build-1234567890"
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnitId = "INVALID"
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: work_unit_id: does not match pattern"))
				})
				t.Run("not prefixed", func(t *ftt.Test) {
					req.WorkUnitId = "u-my-work-unit-id"
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("prefixed, in parent is not prefixed", func(t *ftt.Test) {
					t.Run("valid", func(t *ftt.Test) {
						req.WorkUnitId = "wu-parent:child"
						req.Parent = parentWorkUnitID.Name()
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.WorkUnitId = "otherworkunit:child"
						req.Parent = parentWorkUnitID.Name()
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`bad request: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID "wu-parent"`))
					})
				})
				t.Run("prefixed, in parent that is prefixed", func(t *ftt.Test) {
					t.Run("valid", func(t *ftt.Test) {
						req.WorkUnitId = "wu-parent:child"
						req.Parent = prefixedParentWorkUnitID.Name()
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.WorkUnitId = "otherworkunit:child"
						req.Parent = prefixedParentWorkUnitID.Name()
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`bad request: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID prefix "wu-parent"`))
					})
				})
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("bad request: request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RequestId = "😃"
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("bad request: request_id: does not match"))
				})
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("basic (same-realm) creation", func(t *ftt.Test) {
				t.Run("with update token", func(t *ftt.Test) {
					t.Run("allowed", func(t *ftt.Test) {
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
				})
				t.Run("with inclusion token", func(t *ftt.Test) {
					inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:testrealm")
					assert.Loosely(t, err, should.BeNil)
					ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

					t.Run("allowed", func(t *ftt.Test) {
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:otherrealm")
						assert.Loosely(t, err, should.BeNil)
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

						_, err = recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`desc = work_unit: realm: got realm "testproject:testrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("cross-realm creation", func(t *ftt.Test) {
				req.WorkUnit.Realm = "testproject:otherrealm"

				t.Run("with update token", func(t *ftt.Test) {
					// Provide the permission required.
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permCreateWorkUnit,
					})
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permIncludeWorkUnit,
					})
					t.Run("allowed with required permissions", func(t *ftt.Test) {
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`desc = caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed without include work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeWorkUnit)
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`desc = caller does not have permission "resultdb.workUnits.include" in realm "testproject:otherrealm"`))
					})
				})
				t.Run("with inclusion token", func(t *ftt.Test) {
					// Provide the permission required.
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm:      "testproject:otherrealm",
						Permission: permCreateWorkUnit,
					})

					inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:otherrealm")
					assert.Loosely(t, err, should.BeNil)
					ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

					t.Run("allowed with required permissions", func(t *ftt.Test) {
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`desc = caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						req.WorkUnit.Realm = "testproject:thirdrealm"

						_, err = recorder.CreateWorkUnit(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`desc = work_unit: realm: got realm "testproject:thirdrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("reserved id", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnitWithReservedID)
				req.WorkUnitId = "build-8765432100"

				t.Run("disallowed without realm permission", func(t *ftt.Test) {
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`desc = work_unit_id: only work units created by trusted systems may have id not starting with "u-"`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID,
					})
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("producer resource", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permSetWorkUnitProducerResource)
				req.WorkUnit.ProducerResource = "//builds.example.com/builds/1"
				t.Run("disallowed without permission", func(t *ftt.Test) {
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`desc = work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource,
					})
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("inclusion or update token is validated", func(t *ftt.Test) {
				t.Run("invalid update token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`desc = invalid update token`))
				})
				t.Run("invalid inclusion token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, "invalid-token"))
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`desc = invalid inclusion token`))
				})
				t.Run("missing update and inclusion token", func(t *ftt.Test) {
					// Other test cases are covered by extractInclusionOrUpdateToken.
					ctx := metadata.NewOutgoingContext(ctx, metadata.MD{})
					_, err := recorder.CreateWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`desc = expected either update-token or inclusion-token metadata value in the request`))
				})
			})
		})

		t.Run("parent not active", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
				"RootInvocationShardId": parentWorkUnitID.RootInvocationShardID(),
				"WorkUnitId":            parentWorkUnitID.WorkUnitID,
				"FinalizationState":     pb.WorkUnit_FINALIZING,
			}))

			_, err = recorder.CreateWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.That(t, err, should.ErrLike("desc = parent \"rootInvocations/root-inv-id/workUnits/wu-parent\" is not active"))
		})

		t.Run("parent does not exist", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", parentWorkUnitID.Key()))

			_, err = recorder.CreateWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike("desc = \"rootInvocations/root-inv-id/workUnits/wu-parent\" not found"))
		})

		t.Run("already exists with different request id", func(t *ftt.Test) {
			wu := workunits.NewBuilder(rootInvID, workUnitID.WorkUnitID).WithCreateRequestID("different-request-id").Build()
			testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

			_, err := recorder.CreateWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
			assert.That(t, err, should.ErrLike("desc = \"rootInvocations/root-inv-id/workUnits/wu-new\" already exists"))
		})

		t.Run("create is idempotent", func(t *ftt.Test) {
			res1, err := recorder.CreateWorkUnit(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Send the exact same request again.
			res2, err := recorder.CreateWorkUnit(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, res2, should.Match(res1))
		})

		t.Run("end to end success", func(t *ftt.Test) {
			wuProperties := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
					"key":   structpb.NewStringValue("workunit"),
				},
			}
			instructions := testutil.TestInstructions()
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			req.WorkUnit = &pb.WorkUnit{
				Realm: "testproject:testrealm",
				State: pb.WorkUnit_RUNNING,
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:    "mymodule",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				},
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				ExtendedProperties: extendedProperties,
				Instructions:       instructions,
			}

			expectedWU := proto.Clone(req.WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU, &pb.WorkUnit{ // Merge defaulted and output-only fields.
				Name:              workUnitID.Name(),
				Parent:            parentWorkUnitID.Name(),
				WorkUnitId:        workUnitID.WorkUnitID,
				FinalizationState: pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:           "user:someone@example.com",
				Deadline:          timestamppb.New(start.Add(defaultDeadlineDuration)),
			})
			expectedWU.Instructions = instructionutil.InstructionsWithNames(instructions, workUnitID.Name())
			pbutil.PopulateModuleIdentifierHashes(expectedWU.ModuleId)

			expectWURow := &workunits.WorkUnitRow{
				ID:                workUnitID,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				FinalizationState: pb.WorkUnit_ACTIVE,
				State:             pb.WorkUnit_RUNNING,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
				ModuleID: &pb.ModuleIdentifier{
					ModuleName:        "mymodule",
					ModuleScheme:      "gtest",
					ModuleVariant:     pbutil.Variant("k", "v"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
				},
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				Instructions:       instructionutil.InstructionsWithNames(instructions, workUnitID.Name()),
				ExtendedProperties: extendedProperties,
			}

			expectedLegacyInv := &pb.Invocation{
				Name:      "invocations/workunit:root-inv-id:wu-new",
				State:     pb.Invocation_ACTIVE,
				Realm:     "testproject:testrealm",
				CreatedBy: "user:someone@example.com",
				Deadline:  timestamppb.New(start.Add(defaultDeadlineDuration)),
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:        "mymodule",
					ModuleScheme:      "gtest",
					ModuleVariant:     pbutil.Variant("k", "v"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
				},
				ProducerResource:       "//producer.example.com/builds/123",
				Tags:                   pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:             wuProperties,
				SourceSpec:             &pb.SourceSpec{Inherit: true},
				IsSourceSpecFinal:      true,
				ExtendedProperties:     extendedProperties,
				Instructions:           instructionutil.InstructionsWithNames(instructions, "invocations/workunit:root-inv-id:wu-new"),
				TestResultVariantUnion: pbutil.Variant("k", "v"), // From ModuleId.ModuleVariant.
			}

			t.Run("baseline", func(t *ftt.Test) {
				var headers metadata.MD
				res, err := recorder.CreateWorkUnit(ctx, req, grpc.Header(&headers))
				assert.Loosely(t, err, should.BeNil)

				// Merge server-populated fields for comparison.
				commitTime := res.CreateTime.AsTime()
				proto.Merge(expectedWU, &pb.WorkUnit{
					CreateTime:  timestamppb.New(commitTime),
					LastUpdated: timestamppb.New(commitTime),
					Etag:        fmt.Sprintf(`W/"+f/%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res, should.Match(expectedWU))

				// Check for the new update token in headers.
				token := headers.Get(pb.UpdateTokenMetadataKey)
				assert.Loosely(t, token, should.HaveLength(1))
				assert.Loosely(t, token[0], should.NotBeEmpty)

				// Check the database.
				readCtx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				row, err := workunits.Read(readCtx, workUnitID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectWURow.SecondaryIndexShardID = row.SecondaryIndexShardID
				expectWURow.CreateTime = commitTime
				expectWURow.LastUpdated = commitTime
				assert.That(t, row, should.Match(expectWURow))

				// Check the legacy invocation is inserted.
				legacyInv, err := invocations.Read(readCtx, workUnitID.LegacyInvocationID(), invocations.AllFields)
				expectedLegacyInv.CreateTime = timestamppb.New(commitTime)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, legacyInv, should.Match(expectedLegacyInv))

				// Check inclusion is added to IncludedInvocations.
				includedIDs, err := invocations.ReadIncluded(readCtx, parentWorkUnitID.LegacyInvocationID())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, includedIDs, should.HaveLength(1))
				assert.That(t, includedIDs.Has(workUnitID.LegacyInvocationID()), should.BeTrue)
			})

			t.Run("with state not specified", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_STATE_UNSPECIFIED
				var headers metadata.MD
				res, err := recorder.CreateWorkUnit(ctx, req, grpc.Header(&headers))
				assert.Loosely(t, err, should.BeNil)

				// Merge server-populated fields for comparison.
				commitTime := res.CreateTime.AsTime()
				proto.Merge(expectedWU, &pb.WorkUnit{
					State:       pb.WorkUnit_PENDING,
					CreateTime:  timestamppb.New(commitTime),
					LastUpdated: timestamppb.New(commitTime),
					Etag:        fmt.Sprintf(`W/"+f/%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res, should.Match(expectedWU))

				// Check for the new update token in headers.
				token := headers.Get(pb.UpdateTokenMetadataKey)
				assert.Loosely(t, token, should.HaveLength(1))
				assert.Loosely(t, token[0], should.NotBeEmpty)

				// Check the database.
				readCtx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				row, err := workunits.Read(readCtx, workUnitID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectWURow.SecondaryIndexShardID = row.SecondaryIndexShardID
				expectWURow.State = pb.WorkUnit_PENDING
				expectWURow.CreateTime = commitTime
				expectWURow.LastUpdated = commitTime
				assert.That(t, row, should.Match(expectWURow))
			})
		})
	})
}
