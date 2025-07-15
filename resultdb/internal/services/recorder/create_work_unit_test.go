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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestVerifyCreateWorkUnitPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`VerifyCreateWorkUnitPermissions`, t, func(t *ftt.Test) {
		basePerms := []authtest.RealmPermission{
			{Realm: "project:realm", Permission: permCreateWorkUnit},
			{Realm: "project:realm", Permission: permIncludeWorkUnit},
		}

		authState := &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: basePerms,
		}
		ctx := auth.WithState(context.Background(), authState)

		request := &pb.CreateWorkUnitRequest{
			WorkUnitId: "u-wu",
			WorkUnit: &pb.WorkUnit{
				Realm: "project:realm",
			},
		}

		t.Run("unspecified work unit", func(t *ftt.Test) {
			request.WorkUnit = nil
			err := verifyCreateWorkUnitPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("work_unit: unspecified"))
		})
		t.Run("unspecified realm", func(t *ftt.Test) {
			request.WorkUnit.Realm = ""
			err := verifyCreateWorkUnitPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("work_unit: realm: unspecified"))
		})

		t.Run("invalid realm", func(t *ftt.Test) {
			request.WorkUnit.Realm = "invalid:"
			err := verifyCreateWorkUnitPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`work_unit: realm: bad global realm name`))
		})

		t.Run("basic creation", func(t *ftt.Test) {
			t.Run("allowed", func(t *ftt.Test) {
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("create work unit disallowed", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.create"`))
			})
			t.Run("include work unit disallowed", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeWorkUnit)
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.include"`))
			})
		})

		t.Run("reserved id", func(t *ftt.Test) {
			request.WorkUnitId = "build-8765432100"

			t.Run("disallowed", func(t *ftt.Test) {
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`work_unit_id: only work units created by trusted systems may have id not starting with "u-"`))
			})

			t.Run("allowed with realm permission", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "project:@root", Permission: permCreateWorkUnitWithReservedID,
				})
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("allowed with trusted group", func(t *ftt.Test) {
				authState.IdentityGroups = []string{trustedCreatorGroup}
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("producer resource", func(t *ftt.Test) {
			request.WorkUnit.ProducerResource = "//builds.example.com/builds/1"
			t.Run("disallowed", func(t *ftt.Test) {
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`))
			})

			t.Run("allowed with realm permission", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "project:@root", Permission: permSetWorkUnitProducerResource,
				})
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("allowed with trusted group", func(t *ftt.Test) {
				authState.IdentityGroups = []string{trustedCreatorGroup}
				err := verifyCreateWorkUnitPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestValidateCreateWorkUnitRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	ftt.Run("ValidateCreateWorkUnitRequest", t, func(t *ftt.Test) {
		req := &pb.CreateWorkUnitRequest{
			Parent:     "rootInvocations/u-my-root-id/workUnits/root",
			WorkUnitId: "u-my-work-unit-id",
			WorkUnit: &pb.WorkUnit{
				Realm: "project:realm",
			},
			RequestId: "request-id",
		}
		// This is always true for single create work unit requests,
		// for batch requests it may be false as the request_id can be
		// set on the parent request object.
		requireRequestID := true

		t.Run("valid", func(t *ftt.Test) {
			err := validateCreateWorkUnitRequest(req, requireRequestID)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("parent", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.Parent = ""
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("parent: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.Parent = "invalid"
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("parent: does not match"))
			})
		})
		t.Run("work_unit_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.WorkUnitId = ""
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit_id: unspecified"))
			})
			t.Run("reserved", func(t *ftt.Test) {
				req.WorkUnitId = "build-1234567890"
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnitId = "INVALID"
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit_id: does not match"))
			})

			t.Run("prefix", func(t *ftt.Test) {
				t.Run("not prefixed", func(t *ftt.Test) {
					req.WorkUnitId = "u-my-work-unit-id"
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("parent is prefixed", func(t *ftt.Test) {
					t.Run("valid", func(t *ftt.Test) {
						req.WorkUnitId = "swarming123:a2"
						req.Parent = "rootInvocations/u-my-root-id/workUnits/swarming123:a"
						err := validateCreateWorkUnitRequest(req, requireRequestID)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.WorkUnitId = "swarming2:a2"
						req.Parent = "rootInvocations/u-my-root-id/workUnits/swarming1:a"
						err := validateCreateWorkUnitRequest(req, requireRequestID)
						assert.Loosely(t, err, should.ErrLike("must match parent work unit ID prefix"))
					})
				})
				t.Run("parent is not prefix", func(t *ftt.Test) {
					t.Run("valid", func(t *ftt.Test) {
						req.WorkUnitId = "root:a"
						req.Parent = "rootInvocations/u-my-root-id/workUnits/root"
						err := validateCreateWorkUnitRequest(req, requireRequestID)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.WorkUnitId = "swarming2:a"
						req.Parent = "rootInvocations/u-my-root-id/workUnits/swarming1"
						err := validateCreateWorkUnitRequest(req, requireRequestID)
						assert.Loosely(t, err, should.ErrLike("must match parent work unit ID"))
					})
				})
			})
		})

		t.Run("work_unit", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.WorkUnit = nil
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit: unspecified"))
			})
			t.Run("state", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// If it is unset, we will populate a default value.
					req.WorkUnit.State = pb.WorkUnit_STATE_UNSPECIFIED
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("active", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_ACTIVE
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("finalizing", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_FINALIZING
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.State = pb.WorkUnit_FINALIZED
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: state: cannot be created in the state FINALIZED"))
				})
			})
			t.Run("realm", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.WorkUnit.Realm = ""
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: realm: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Realm = "invalid:"
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: realm: bad global realm name"))
				})
			})
			t.Run("deadline", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// Empty is valid, the deadline will be defaulted.
					req.WorkUnit.Deadline = nil
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: deadline: must be at least 10 seconds in the future"))
				})
			})
			t.Run("producer_resource", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = ""
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = "//cr-buildbucket.appspot.com/builds/1234567890"
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.ProducerResource = "invalid"
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: producer_resource: resource name \"invalid\" does not start with '//'"))
				})
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Tags = nil
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike(`work_unit: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.WorkUnit.Tags = tags
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Properties = nil
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike(`work_unit: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.WorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("extended_properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.ExtendedProperties = nil
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid key", func(t *ftt.Test) {
					req.WorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					req.WorkUnit.ExtendedProperties["invalid_key@"] = &structpb.Struct{}
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike(`work_unit: extended_properties: key "invalid_key@"`))
				})
			})
			t.Run("instructions", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.WorkUnit.Instructions = nil
					err := validateCreateWorkUnitRequest(req, requireRequestID)
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
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.WorkUnit.Instructions = &pb.Instructions{
						Instructions: []*pb.Instruction{
							{},
						},
					}
					err := validateCreateWorkUnitRequest(req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("work_unit: instructions: instructions[0]: id: unspecified"))
				})
			})
		})
		t.Run("request_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
			})
			t.Run("empty but not required", func(t *ftt.Test) {
				requireRequestID = false
				req.RequestId = ""
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateCreateWorkUnitRequest(req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
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
				token, err := generateWorkUnitToken(ctx, id)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)

				err = validateWorkUnitToken(ctx, token, id)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("work unit ID prefixed", func(t *ftt.Test) {
				idWithPrefix := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "base:a2",
				}
				token, err := generateWorkUnitToken(ctx, idWithPrefix)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, token, should.NotBeEmpty)

				idWithPrefix.WorkUnitID = "base:a1"
				err = validateWorkUnitToken(ctx, token, idWithPrefix)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("workUnitTokenState", func(t *ftt.Test) {
			t.Run("work unit id not prefixed", func(t *ftt.Test) {
				id := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "work-unit-id",
				}
				assert.Loosely(t, workUnitTokenState(id), should.Equal(`"root-inv-id":"work-unit-id"`))
			})
			t.Run("work unit ID prefixed", func(t *ftt.Test) {
				id := workunits.ID{
					RootInvocationID: "root-inv-id",
					WorkUnitID:       "base:a2",
				}
				assert.Loosely(t, workUnitTokenState(id), should.Equal(`"root-inv-id":"base"`))
			})
		})
	})
}
