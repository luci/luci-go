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
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestVerifyCreateRootInvocationPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`VerifyCreateRootInvocationPermissions`, t, func(t *ftt.Test) {
		basePerms := []authtest.RealmPermission{
			{Realm: "project:realm", Permission: permCreateRootInvocation},
			{Realm: "project:realm", Permission: permCreateWorkUnit},
		}

		authState := &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: basePerms,
		}
		ctx := auth.WithState(context.Background(), authState)

		request := &pb.CreateRootInvocationRequest{
			RootInvocationId: "u-inv",
			RootInvocation: &pb.RootInvocation{
				Realm: "project:realm",
			},
		}
		t.Run("unspecified root invocation", func(t *ftt.Test) {
			request.RootInvocation = nil
			err := verifyCreateRootInvocationPermissions(ctx, request)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("root_invocation: unspecified"))
		})
		t.Run("unspecified realm", func(t *ftt.Test) {
			request.RootInvocation.Realm = ""
			err := verifyCreateRootInvocationPermissions(ctx, &pb.CreateRootInvocationRequest{
				RootInvocationId: "u-inv",
				RootInvocation:   &pb.RootInvocation{},
			})
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("root_invocation: realm: unspecified"))
		})

		t.Run("invalid realm", func(t *ftt.Test) {
			request.RootInvocation.Realm = "invalid:"
			err := verifyCreateRootInvocationPermissions(ctx, &pb.CreateRootInvocationRequest{
				RootInvocationId: "u-inv",
				RootInvocation: &pb.RootInvocation{
					Realm: "invalid:",
				},
			})
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`root_invocation: realm: bad global realm name`))
		})

		t.Run("basic creation", func(t *ftt.Test) {
			t.Run("allowed", func(t *ftt.Test) {
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("create root invocation disallowed", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateRootInvocation)
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.rootInvocations.create"`))
			})
			t.Run("create work unit disallowed", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission "resultdb.workUnits.create"`))
			})
		})

		t.Run("reserved id", func(t *ftt.Test) {
			request.RootInvocationId = "build-8765432100"

			t.Run("disallowed", func(t *ftt.Test) {
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`only root invocations created by trusted systems may have id not starting with "u-"`))
			})

			t.Run("allowed with realm permission", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "project:@root", Permission: permCreateRootInvocationWithReservedID,
				})
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("allowed with trusted group", func(t *ftt.Test) {
				authState.IdentityGroups = []string{trustedCreatorGroup}
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("producer resource", func(t *ftt.Test) {
			request.RootInvocation.ProducerResource = "//builds.example.com/builds/1"
			t.Run("disallowed", func(t *ftt.Test) {
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`only root invocations created by trusted system may have a populated producer_resource field`))
			})

			t.Run("allowed with realm permission", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "project:@root", Permission: permSetRootInvocationProducerResource,
				})
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("allowed with trusted group", func(t *ftt.Test) {
				authState.IdentityGroups = []string{trustedCreatorGroup}
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("baseline", func(t *ftt.Test) {
			request.RootInvocation.BaselineId = "try:linux-rel"

			t.Run("disallowed", func(t *ftt.Test) {
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`caller does not have permission to write to test baseline`))
			})

			t.Run("allowed", func(t *ftt.Test) {
				authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
					Realm: "project:@project", Permission: permPutBaseline,
				})
				err := verifyCreateRootInvocationPermissions(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func TestValidateCreateRootInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	ftt.Run("ValidateCreateRootInvocationRequest", t, func(t *ftt.Test) {
		req := &pb.CreateRootInvocationRequest{
			RootInvocationId: "u-my-root-id",
			RootInvocation: &pb.RootInvocation{
				Realm: "project:realm",
			},
			RootWorkUnit: &pb.WorkUnit{},
			RequestId:    "request-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateCreateRootInvocationRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("root_invocation_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RootInvocationId = ""
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_invocation_id: unspecified"))
			})
			t.Run("reserved", func(t *ftt.Test) {
				req.RootInvocationId = "build-1234567890"
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocationId = "INVALID"
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_invocation_id: does not match"))
			})
		})

		t.Run("root_invocation", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RootInvocation = nil
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_invocation: unspecified"))
			})
			t.Run("state", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// If it is unset, we will populate a default value.
					req.RootInvocation.State = pb.RootInvocation_STATE_UNSPECIFIED
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("active", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_ACTIVE
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("finalizing", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_FINALIZING
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_FINALIZED
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: state: cannot be created in the state FINALIZED"))
				})
			})
			t.Run("realm", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RootInvocation.Realm = ""
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: realm: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Realm = "invalid:"
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: realm: bad global realm name"))
				})
			})
			t.Run("deadline", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// Empty is valid, the deadline will be defaulted.
					req.RootInvocation.Deadline = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: deadline: must be at least 10 seconds in the future"))
				})
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Tags = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike(`root_invocation: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.RootInvocation.Tags = tags
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("producer_resource", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = ""
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = "//cr-buildbucket.appspot.com/builds/1234567890"
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = "invalid"
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: producer_resource: resource name \"invalid\" does not start with '//'"))
				})
			})
			t.Run("sources", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Sources = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Sources = &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "host",
							Project:    "project",
							Ref:        "refs/heads/main",
							CommitHash: "0123456789012345678901234567890123456789",
							Position:   5,
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Sources = &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: sources: gitiles_commit: host: unspecified"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Properties = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike(`root_invocation: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("baseline_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.BaselineId = ""
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.BaselineId = "try/linux-rel"
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_invocation: baseline_id: does not match"))
				})
			})
		})
		t.Run("root_work_unit", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RootWorkUnit = nil
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: unspecified"))
			})
			t.Run("state", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.State = pb.WorkUnit_ACTIVE
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: state: must not be set; always inherited from root invocation"))
			})
			t.Run("realm", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.Realm = "project:realm"
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: realm: must not be set"))
			})
			t.Run("deadline", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: deadline: must not be set; always inherited from root invocation"))
			})
			t.Run("producer resource", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.ProducerResource = "//chromium-swarm.appspot.com/tasks/deadbeef"
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: producer_resource: must not be set; always inherited from root invocation"))
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.RootWorkUnit.Tags = tags
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("extended_properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.ExtendedProperties = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid key", func(t *ftt.Test) {
					req.RootWorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					req.RootWorkUnit.ExtendedProperties["invalid_key@"] = &structpb.Struct{}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: extended_properties: key "invalid_key@"`))
				})
			})
			t.Run("instructions", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Instructions = nil
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.Instructions = &pb.Instructions{
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
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Instructions = &pb.Instructions{
						Instructions: []*pb.Instruction{
							{},
						},
					}
					err := validateCreateRootInvocationRequest(req)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: instructions: instructions[0]: id: unspecified"))
				})
			})
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateCreateRootInvocationRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})
	})
}

func TestValidateDeadline(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateDeadline`, t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC

		t.Run(`deadline in the past`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be at least 10 seconds in the future`))
		})

		t.Run(`deadline 5s in the future`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(5 * time.Second))
			err := validateDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be at least 10 seconds in the future`))
		})

		t.Run(`deadline too far in the future`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(1e3 * time.Hour))
			err := validateDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be before 120h in the future`))
		})

		t.Run(`valid`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
			err := validateDeadline(deadline, now)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}
