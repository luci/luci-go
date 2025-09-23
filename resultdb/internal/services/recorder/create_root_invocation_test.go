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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
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
		// Construct a valid request.
		req := &pb.CreateRootInvocationRequest{
			RootInvocationId: "u-my-root-id",
			RootInvocation: &pb.RootInvocation{
				Realm: "project:realm",
			},
			RootWorkUnit: &pb.WorkUnit{},
			RequestId:    "request-id",
		}

		cfg, err := config.NewCompiledServiceConfig(config.CreatePlaceHolderServiceConfig(), "revision")
		assert.NoErr(t, err)

		t.Run("valid", func(t *ftt.Test) {
			err := validateCreateRootInvocationRequest(req, cfg)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("root_invocation_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RootInvocationId = ""
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_invocation_id: unspecified"))
			})
			t.Run("reserved", func(t *ftt.Test) {
				req.RootInvocationId = "build-1234567890"
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocationId = "INVALID"
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_invocation_id: does not match"))
			})
		})

		t.Run("root_invocation", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RootInvocation = nil
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_invocation: unspecified"))
			})
			t.Run("state", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// If it is unset, we will populate a default value.
					req.RootInvocation.State = pb.RootInvocation_STATE_UNSPECIFIED
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("active", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_ACTIVE
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("finalizing", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_FINALIZING
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.State = pb.RootInvocation_FINALIZED
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: state: cannot be created in the state FINALIZED"))
				})
			})
			t.Run("realm", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RootInvocation.Realm = ""
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: realm: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Realm = "invalid:"
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: realm: bad global realm name"))
				})
			})
			t.Run("deadline", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					// Empty is valid, the deadline will be defaulted.
					req.RootInvocation.Deadline = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: deadline: must be at least 10 seconds in the future"))
				})
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Tags = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_invocation: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.RootInvocation.Tags = tags
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("producer_resource", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = ""
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = "//cr-buildbucket.appspot.com/builds/1234567890"
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.ProducerResource = "invalid"
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: producer_resource: resource name \"invalid\" does not start with '//'"))
				})
			})
			t.Run("sources", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Sources = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Sources = &pb.Sources{
						BaseSources: &pb.Sources_GitilesCommit{
							GitilesCommit: &pb.GitilesCommit{
								Host:       "host",
								Project:    "project",
								Ref:        "refs/heads/main",
								CommitHash: "0123456789012345678901234567890123456789",
								Position:   5,
							},
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Sources = &pb.Sources{
						BaseSources: &pb.Sources_GitilesCommit{
							GitilesCommit: &pb.GitilesCommit{},
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: sources: gitiles_commit: host: unspecified"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.Properties = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_invocation: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.RootInvocation.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("baseline_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootInvocation.BaselineId = ""
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootInvocation.BaselineId = "try/linux-rel"
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_invocation: baseline_id: does not match"))
				})
			})
		})
		t.Run("root_work_unit", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RootWorkUnit = nil
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: unspecified"))
			})
			t.Run("state", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.State = pb.WorkUnit_ACTIVE
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: state: must not be set; always inherited from root invocation"))
			})
			t.Run("realm", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.Realm = "project:realm"
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: realm: must not be set"))
			})
			t.Run("deadline", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: deadline: must not be set; always inherited from root invocation"))
			})
			t.Run("module_id", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					// This is valid.
					req.RootWorkUnit.ModuleId = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "gtest", // This is in the service config we use for testing.
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("structurally invalid", func(t *ftt.Test) {
					// pbutil.ValidateModuleIdentifierForStorage has its own
					// exhaustive tests, verify it is being called.
					req.RootWorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:        "mymodule",
						ModuleScheme:      "gtest",
						ModuleVariantHash: "aaaaaaaaaaaaaaaa", // Variant hash only is not allowed for storage.
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: module_id: module_variant: unspecified"))
				})
				t.Run("invalid with respect to service configuration", func(t *ftt.Test) {
					req.RootWorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "cooltest", // This is not defined in the service config.
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: module_id: module_scheme: scheme "cooltest" is not a known scheme by the ResultDB deployment; see go/resultdb-schemes for instructions how to define a new scheme`))
				})
			})
			t.Run("producer resource", func(t *ftt.Test) {
				// Must not be set.
				req.RootWorkUnit.ProducerResource = "//chromium-swarm.appspot.com/tasks/deadbeef"
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("root_work_unit: producer_resource: must not be set; always inherited from root invocation"))
			})
			t.Run("tags", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = pbutil.StringPairs("key", "value")
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Tags = pbutil.StringPairs("1", "a")
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: tags: "1":"a": key: does not match`))
				})
				t.Run("too large", func(t *ftt.Test) {
					tags := make([]*pb.StringPair, 51)
					for i := 0; i < 51; i++ {
						tags[i] = pbutil.StringPair(strings.Repeat("k", 64), strings.Repeat("v", 256))
					}
					req.RootWorkUnit.Tags = tags
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: tags: got 16575 bytes; exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("valid", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: properties: must have a field "@type"`))
				})
				t.Run("too large", func(t *ftt.Test) {
					req.RootWorkUnit.Properties = &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"a":     structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: properties: the size of properties (16448) exceeds the maximum size of 16384 bytes"))
				})
			})
			t.Run("extended_properties", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.ExtendedProperties = nil
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid key", func(t *ftt.Test) {
					req.RootWorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					req.RootWorkUnit.ExtendedProperties["invalid_key@"] = &structpb.Struct{}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike(`root_work_unit: extended_properties: key "invalid_key@"`))
				})
			})
			t.Run("instructions", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.RootWorkUnit.Instructions = nil
					err := validateCreateRootInvocationRequest(req, cfg)
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
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RootWorkUnit.Instructions = &pb.Instructions{
						Instructions: []*pb.Instruction{
							{},
						},
					}
					err := validateCreateRootInvocationRequest(req, cfg)
					assert.Loosely(t, err, should.ErrLike("root_work_unit: instructions: instructions[0]: id: unspecified"))
				})
			})
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateCreateRootInvocationRequest(req, cfg)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified (please provide a per-request UUID to ensure idempotence)"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateCreateRootInvocationRequest(req, cfg)
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

func TestCreateRootInvocation(t *testing.T) {
	ftt.Run(`TestCreateRootInvocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.
		err := config.SetServiceConfigForTesting(ctx, config.CreatePlaceHolderServiceConfig())
		assert.NoErr(t, err)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permCreateRootInvocation},
				{Realm: "testproject:testrealm", Permission: permCreateWorkUnit},
				{Realm: "testproject:@root", Permission: permCreateRootInvocationWithReservedID},
				{Realm: "testproject:@project", Permission: permPutBaseline},
				{Realm: "testproject:@root", Permission: permSetRootInvocationProducerResource},
			},
		})

		// set test clock
		start := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, start)

		// Setup a full HTTP server in order to retrieve response headers.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		req := &pb.CreateRootInvocationRequest{
			RootInvocationId: "root-inv-id",
			RootInvocation:   &pb.RootInvocation{Realm: "testproject:testrealm"},
			RootWorkUnit:     &pb.WorkUnit{},
			RequestId:        "request-id",
		}

		t.Run("invalid request", func(t *ftt.Test) {
			// Request validation exhaustively tested in test cases for validateCreateRootInvocationRequest.
			// These tests only exist to ensure that method is called.

			t.Run("empty request", func(t *ftt.Test) {
				_, err := recorder.CreateRootInvocation(ctx, &pb.CreateRootInvocationRequest{})
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("root_invocation: unspecified"))
			})

			t.Run("invalid root invocation id", func(t *ftt.Test) {
				req.RootInvocationId = "invalid id"

				_, err := recorder.CreateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("root_invocation_id: does not match"))
			})

			t.Run("missing realm", func(t *ftt.Test) {
				req.RootInvocation.Realm = ""

				_, err := recorder.CreateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("realm: unspecified"))
			})

			t.Run("root work unit invalid", func(t *ftt.Test) {
				req.RootWorkUnit.Realm = "secretproject:testrealm"

				_, err := recorder.CreateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("root_work_unit: realm: must not be set; always inherited from root invocation"))
			})
		})

		// Request authorisation exhaustively tested in test cases for verifyCreateRootInvocationPermissions.
		// This test case only exists to verify that method is called.
		t.Run("permission denied for create", func(t *ftt.Test) {
			req.RootInvocation.Realm = "secretproject:testrealm"

			_, err := recorder.CreateRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("caller does not have permission \"resultdb.rootInvocations.create\" in realm \"secretproject:testrealm\""))
		})
		t.Run("already exists with different request id", func(t *ftt.Test) {
			testData := rootinvocations.NewBuilder("u-already-exists").Build()
			testutil.MustApply(ctx, t, insert.RootInvocationOnly(testData)...)
			req.RootInvocationId = "u-already-exists"

			_, err := recorder.CreateRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
			assert.That(t, err, should.ErrLike("rootInvocations/u-already-exists already exists"))
		})

		t.Run("create is idempotent", func(t *ftt.Test) {
			res1, err := recorder.CreateRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Send the exact same request again.
			res2, err := recorder.CreateRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, res2, should.Match(res1))
		})

		t.Run("end to end success", func(t *ftt.Test) {
			// Set all fields helps optimise test coverage.
			invProperties := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
					"key":   structpb.NewStringValue("rootinvocation"),
				},
			}
			wuProperties := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
					"key":   structpb.NewStringValue("workunit"),
				},
			}
			extendedProperties := map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			}
			instructions := testutil.TestInstructions()
			workUnitTags := pbutil.StringPairs("wu_key", "wu_value")
			invTags := pbutil.StringPairs("tag_key", "tag_value")
			sources := &pb.Sources{
				BaseSources: &pb.Sources_GitilesCommit{
					GitilesCommit: &pb.GitilesCommit{
						Host:       "chromium.googlesource.com",
						Project:    "chromium/src",
						Ref:        "refs/heads/main",
						CommitHash: "1234567890abcdef1234567890abcdef12345678",
						Position:   12345,
					},
				},
			}
			req := &pb.CreateRootInvocationRequest{
				RootInvocationId: "u-e2e-success",
				RequestId:        "e2e-request",
				RootInvocation: &pb.RootInvocation{
					Realm:            "testproject:testrealm",
					ProducerResource: "//builds.example.com/builds/1",
					Sources:          sources,
					SourcesFinal:     true,
					Tags:             invTags,
					Properties:       invProperties,
					BaselineId:       "testrealm:test-builder",
				},
				RootWorkUnit: &pb.WorkUnit{
					ModuleId: &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "gtest",
						ModuleVariant: pbutil.Variant("k", "v"),
					},
					Tags:               workUnitTags,
					Properties:         wuProperties,
					ExtendedProperties: extendedProperties,
					Instructions:       instructions,
				},
			}

			// Expected Response.
			expectedInv := proto.Clone(req.RootInvocation).(*pb.RootInvocation)
			proto.Merge(expectedInv, &pb.RootInvocation{ // Merge defaulted and output-only fields.
				Name:             "rootInvocations/u-e2e-success",
				RootInvocationId: "u-e2e-success",
				State:            pb.RootInvocation_ACTIVE, // State is defaulted to ACTIVE.
				Creator:          "user:someone@example.com",
				Deadline:         timestamppb.New(start.Add(defaultDeadlineDuration)),
			})
			wuID := workunits.ID{
				RootInvocationID: "u-e2e-success",
				WorkUnitID:       "root",
			}
			expectedWU := proto.Clone(req.RootWorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU, &pb.WorkUnit{ // Merge defaulted and output-only fields.
				Name:             "rootInvocations/u-e2e-success/workUnits/root",
				Parent:           "rootInvocations/u-e2e-success",
				WorkUnitId:       "root",
				State:            pb.WorkUnit_ACTIVE,
				Realm:            "testproject:testrealm",
				Creator:          "user:someone@example.com",
				Deadline:         timestamppb.New(start.Add(defaultDeadlineDuration)),
				ProducerResource: "//builds.example.com/builds/1",
			})
			expectedWU.Instructions = instructionutil.InstructionsWithNames(instructions, wuID.Name())
			pbutil.PopulateModuleIdentifierHashes(expectedWU.ModuleId)

			rootInvocationID := rootinvocations.ID("u-e2e-success")
			expectInvRow := &rootinvocations.RootInvocationRow{
				RootInvocationID:                        rootInvocationID,
				State:                                   pb.RootInvocation_ACTIVE,
				Realm:                                   "testproject:testrealm",
				CreatedBy:                               "user:someone@example.com",
				FinalizeStartTime:                       spanner.NullTime{},
				FinalizeTime:                            spanner.NullTime{},
				Deadline:                                start.Add(defaultDeadlineDuration),
				UninterestingTestVerdictsExpirationTime: spanner.NullTime{Valid: true, Time: start.Add(expectedResultExpiration)},
				CreateRequestID:                         "e2e-request",
				ProducerResource:                        "//builds.example.com/builds/1",
				Tags:                                    invTags,
				Properties:                              invProperties,
				Sources:                                 sources,
				IsSourcesFinal:                          true,
				BaselineID:                              "testrealm:test-builder",
				Submitted:                               false,
			}

			expectWURow := &workunits.WorkUnitRow{
				ID:                wuID,
				ParentWorkUnitID:  spanner.NullString{Valid: false},
				State:             pb.WorkUnit_ACTIVE,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "e2e-request",
				ModuleID: &pb.ModuleIdentifier{
					ModuleName:        "mymodule",
					ModuleScheme:      "gtest",
					ModuleVariant:     pbutil.Variant("k", "v"),
					ModuleVariantHash: pbutil.VariantHash(pbutil.Variant("k", "v")),
				},
				ProducerResource:   "//builds.example.com/builds/1",
				Tags:               pbutil.StringPairs("wu_key", "wu_value"),
				Properties:         wuProperties,
				Instructions:       instructionutil.InstructionsWithNames(instructions, wuID.Name()),
				ExtendedProperties: extendedProperties,
			}

			t.Run("active root invocation", func(t *ftt.Test) {
				var headers metadata.MD
				res, err := recorder.CreateRootInvocation(ctx, req, grpc.Header(&headers))
				assert.Loosely(t, err, should.BeNil)
				commitTime := res.RootInvocation.CreateTime.AsTime()
				proto.Merge(expectedInv, &pb.RootInvocation{
					CreateTime:  timestamppb.New(commitTime),
					LastUpdated: timestamppb.New(commitTime),
					Etag:        fmt.Sprintf(`W/"%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res.RootInvocation, should.Match(expectedInv))
				proto.Merge(expectedWU, &pb.WorkUnit{
					CreateTime:  timestamppb.New(commitTime),
					LastUpdated: timestamppb.New(commitTime),
					Etag:        fmt.Sprintf(`W/"+f/%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res.RootWorkUnit, should.Match(expectedWU))

				// Check the update token in headers.
				token := headers.Get(pb.UpdateTokenMetadataKey)
				assert.Loosely(t, token, should.HaveLength(1))
				assert.Loosely(t, token[0], should.NotBeEmpty)

				// Check the database.
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				row, err := rootinvocations.Read(ctx, rootInvocationID)
				assert.Loosely(t, err, should.BeNil)
				expectInvRow.SecondaryIndexShardID = row.SecondaryIndexShardID
				expectInvRow.CreateTime = commitTime
				expectInvRow.LastUpdated = commitTime
				assert.That(t, row, should.Match(expectInvRow))

				wuRow, err := workunits.Read(ctx, wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectWURow.SecondaryIndexShardID = wuRow.SecondaryIndexShardID
				expectWURow.CreateTime = commitTime
				expectWURow.LastUpdated = commitTime
				assert.That(t, wuRow, should.Match(expectWURow))

				// Check inclusion is added to IncludedInvocations.
				includedIDs, err := invocations.ReadIncluded(ctx, rootInvocationID.LegacyInvocationID())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, includedIDs, should.HaveLength(1))
				assert.That(t, includedIDs.Has(wuID.LegacyInvocationID()), should.BeTrue)
			})

			t.Run("finalizing root invocation", func(t *ftt.Test) {
				req.RootInvocation.State = pb.RootInvocation_FINALIZING

				var headers metadata.MD
				res, err := recorder.CreateRootInvocation(ctx, req, grpc.Header(&headers))
				assert.Loosely(t, err, should.BeNil)
				commitTime := res.RootInvocation.CreateTime.AsTime()
				proto.Merge(expectedInv, &pb.RootInvocation{
					State:             pb.RootInvocation_FINALIZING,
					CreateTime:        timestamppb.New(commitTime),
					LastUpdated:       timestamppb.New(commitTime),
					FinalizeStartTime: timestamppb.New(commitTime),
					Etag:              fmt.Sprintf(`W/"%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res.RootInvocation, should.Match(expectedInv))
				proto.Merge(expectedWU, &pb.WorkUnit{
					State:             pb.WorkUnit_FINALIZING,
					CreateTime:        timestamppb.New(commitTime),
					LastUpdated:       timestamppb.New(commitTime),
					FinalizeStartTime: timestamppb.New(commitTime),
					Etag:              fmt.Sprintf(`W/"+f/%s"`, commitTime.UTC().Format(time.RFC3339Nano)),
				})
				assert.That(t, res.RootWorkUnit, should.Match(expectedWU))

				// Check the update token in headers.
				token := headers.Get(pb.UpdateTokenMetadataKey)
				assert.Loosely(t, token, should.HaveLength(1))
				assert.Loosely(t, token[0], should.NotBeEmpty)

				// Check the database.
				ctx, cancel := span.ReadOnlyTransaction(ctx)
				defer cancel()
				row, err := rootinvocations.Read(ctx, rootInvocationID)
				assert.Loosely(t, err, should.BeNil)
				expectInvRow.SecondaryIndexShardID = row.SecondaryIndexShardID
				expectInvRow.State = pb.RootInvocation_FINALIZING
				expectInvRow.CreateTime = commitTime
				expectInvRow.LastUpdated = commitTime
				expectInvRow.FinalizeStartTime = spanner.NullTime{Time: commitTime, Valid: true}
				assert.That(t, row, should.Match(expectInvRow))
				// Check finalize start time is set.
				assert.That(t, row.FinalizeStartTime.Valid, should.BeTrue)

				wuRow, err := workunits.Read(ctx, wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				expectWURow.SecondaryIndexShardID = wuRow.SecondaryIndexShardID
				expectWURow.State = pb.WorkUnit_FINALIZING
				expectWURow.CreateTime = commitTime
				expectWURow.LastUpdated = commitTime
				expectWURow.FinalizeStartTime = spanner.NullTime{Time: commitTime, Valid: true}
				assert.That(t, wuRow, should.Match(expectWURow))
				// Check finalize start time is set.
				assert.That(t, wuRow.FinalizeStartTime.Valid, should.BeTrue)

				// Check inclusion is added to IncludedInvocations.
				includedIDs, err := invocations.ReadIncluded(ctx, rootInvocationID.LegacyInvocationID())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, includedIDs, should.HaveLength(1))
				assert.That(t, includedIDs.Has(wuID.LegacyInvocationID()), should.BeTrue)
			})
		})
	})
}
