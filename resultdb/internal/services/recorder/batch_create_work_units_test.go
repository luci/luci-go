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

func TestBatchCreateWorkUnits(t *testing.T) {
	ftt.Run(`TestBatchCreateWorkUnits`, t, func(t *ftt.Test) {
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

		// Setup a full HTTP server.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		rootInvID := rootinvocations.ID("root-inv-id")
		parentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent",
		}
		prefixedParentWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:parent",
		}
		workUnitID1 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:wu-new-1",
		}
		workUnitID11 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:wu-new-11",
		}
		workUnitID111 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new-111",
		}

		// Insert a root invocation and the parent work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
		parentWu := workunits.NewBuilder(rootInvID, parentWorkUnitID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).WithModuleID(nil).Build()
		parentPrefixedWu := workunits.NewBuilder(rootInvID, prefixedParentWorkUnitID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentWu)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentPrefixedWu)...)

		// Generate an update token for the parent work unit.
		parentUpdateToken, err := generateWorkUnitUpdateToken(ctx, parentWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))

		// A basic valid request.
		// parent
		//  |---wu-parent:wu-new-1
		//  |          |--- wu-new-11
		//  |---wu-new-2
		req := &pb.BatchCreateWorkUnitsRequest{
			RequestId: "test-request-id",
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     parentWorkUnitID.Name(),
					WorkUnitId: workUnitID1.WorkUnitID,
					WorkUnit: &pb.WorkUnit{
						Kind:  "EXAMPLE_MODULE",
						State: pb.WorkUnit_PENDING,
						Realm: "testproject:testrealm",
					},
					RequestId: "test-request-id",
				},
				{
					Parent:     workUnitID1.Name(),
					WorkUnitId: workUnitID11.WorkUnitID,
					WorkUnit: &pb.WorkUnit{
						Kind:  "EXAMPLE_MODULE",
						State: pb.WorkUnit_PENDING,
						Realm: "testproject:testrealm",
					},
				},
				{
					Parent:     workUnitID11.Name(),
					WorkUnitId: workUnitID111.WorkUnitID,
					WorkUnit: &pb.WorkUnit{
						Kind:  "EXAMPLE_TEST_RUN",
						State: pb.WorkUnit_PENDING,
						Realm: "testproject:testrealm",
					},
				},
			},
		}

		// The validation on this method is fairly involved. To ensure no cases are missed,
		// we do not test the main validation methods separately but do exhaustive testing on the
		// integrated RPC.

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("no requests", func(t *ftt.Test) {
				req.Requests = nil
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
			})
			t.Run("too many requests", func(t *ftt.Test) {
				req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
			})
			t.Run("total size of requests is too large", func(t *ftt.Test) {
				req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("requests: the size of all requests is too large"))
			})
			t.Run("sub-requests", func(t *ftt.Test) {
				t.Run("parent", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].Parent = ""
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: unspecified"))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[1].Parent = "\x00"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: parent: does not match pattern"))
					})
					t.Run("parent in different root invocation", func(t *ftt.Test) {
						req.Requests[2].Parent = "rootInvocations/another-root/workUnits/root"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[2]: parent: all requests must be for the same root invocation"))
					})
					t.Run("parent cycle", func(t *ftt.Test) {
						t.Run("cycle of length 1", func(t *ftt.Test) {
							// Requests[0] -> Requests[0]
							req.Requests[0].Parent = workUnitID1.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[0]: parent: cannot refer to the work unit created in requests[0]"))
						})
						t.Run("cycle of length 2", func(t *ftt.Test) {
							// Requests[0] -> Requests[1] -> Requests[0]
							req.Requests[0].Parent = workUnitID11.Name()
							req.Requests[1].Parent = workUnitID1.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[0]: parent: cannot refer to work unit created in the later request requests[1], please order requests to match expected creation order"))
						})
					})
				})
				t.Run("work unit id", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req.Requests[2].WorkUnitId = ""
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: unspecified`))
					})
					t.Run("invalid", func(t *ftt.Test) {
						req.Requests[2].WorkUnitId = "\x00"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: does not match pattern`))
					})
					t.Run("duplicated work unit id", func(t *ftt.Test) {
						req.Requests[2].WorkUnitId = req.Requests[0].WorkUnitId
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: duplicates work unit id "wu-parent:wu-new-1" from requests[0]`))
					})
					t.Run("reserved", func(t *ftt.Test) {
						req.Requests[2].WorkUnitId = "build-1234567890"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("not prefixed", func(t *ftt.Test) {
						req.Requests[2].WorkUnitId = "u-my-work-unit-id"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("prefixed, in parent is not prefixed", func(t *ftt.Test) {
						t.Run("valid", func(t *ftt.Test) {
							req.Requests[2].WorkUnitId = "wu-parent:child"
							req.Requests[2].Parent = parentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[2].WorkUnitId = "otherworkunit:child"
							req.Requests[2].Parent = parentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID "wu-parent"`))
						})
					})
					t.Run("prefixed, in parent that is prefixed", func(t *ftt.Test) {
						t.Run("valid", func(t *ftt.Test) {
							req.Requests[2].WorkUnitId = "wu-parent:child"
							req.Requests[2].Parent = prefixedParentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.Loosely(t, err, should.BeNil)
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[2].WorkUnitId = "otherworkunit:child"
							req.Requests[2].Parent = prefixedParentWorkUnitID.Name()
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: work unit ID prefix "otherworkunit" must match parent work unit ID prefix "wu-parent"`))
						})
					})
				})
				t.Run("work unit", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						req.Requests[1].WorkUnit = nil
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: unspecified"))
					})
					t.Run("realm", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.Realm = ""
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: realm: unspecified"))
						})
						t.Run("invalid", func(t *ftt.Test) {
							req.Requests[1].WorkUnit.Realm = "invalid:"
							_, err := recorder.BatchCreateWorkUnits(ctx, req)
							assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: realm: bad global realm name"))
						})
					})
					t.Run("other invalid", func(t *ftt.Test) {
						// validateCreateWorkUnitRequest has its own exhaustive test cases,
						// simply check that it is called.
						req.Requests[1].WorkUnit.State = pb.WorkUnit_FINAL_STATE_MASK
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit: state: FINAL_STATE_MASK is not a valid state"))
					})
				})
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.RequestId = ""
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.RequestId = "😃"
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
				})
				t.Run("mismatched child request id", func(t *ftt.Test) {
					req.Requests[1].RequestId = "another-request-id"
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("requests[1]: request_id: inconsistent with top-level request_id"))
				})
				t.Run("matched child request id", func(t *ftt.Test) {
					req.RequestId = "test-request-id"
					req.Requests[1].RequestId = "test-request-id"

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("basic (same-realm) creation", func(t *ftt.Test) {
				t.Run("with update token", func(t *ftt.Test) {
					t.Run("allowed", func(t *ftt.Test) {
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, parentUpdateToken))
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("requests requiring different update tokens", func(t *ftt.Test) {
						req.Requests[2].Parent = "rootInvocations/root-inv-id/workUnits/other-work-unit"
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[2]: parent "rootInvocations/root-inv-id/workUnits/other-work-unit" requires a different update token to requests[0]'s "parent" "rootInvocations/root-inv-id/workUnits/wu-parent", but this RPC only accepts one update token`))
					})
				})
				t.Run("with inclusion token", func(t *ftt.Test) {
					inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:testrealm")
					assert.Loosely(t, err, should.BeNil)
					ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

					// Use the same parent on all requests, inclusion tokens do not extend to inclusions
					// in other work units sharing the same prefix.
					for _, r := range req.Requests {
						r.Parent = parentWorkUnitID.Name()
					}

					t.Run("allowed", func(t *ftt.Test) {
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("parent requires different inclusion token", func(t *ftt.Test) {
						req.Requests[1].Parent = prefixedParentWorkUnitID.Name()
						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: parent "rootInvocations/root-inv-id/workUnits/wu-parent:parent" requires a different inclusion token to requests[0].parent "rootInvocations/root-inv-id/workUnits/wu-parent", but this RPC only accepts one inclusion token`))
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						inclusionToken, err := generateWorkUnitInclusionToken(ctx, parentWorkUnitID, "testproject:otherrealm")
						assert.Loosely(t, err, should.BeNil)
						ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, inclusionToken))

						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: work_unit: realm: got realm "testproject:testrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("cross-realm creation", func(t *ftt.Test) {
				req.Requests[1].WorkUnit.Realm = "testproject:otherrealm"
				req.Requests[2].WorkUnit.Realm = "testproject:otherrealm"

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
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed without include work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.include" in realm "testproject:otherrealm"`))
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

					for _, r := range req.Requests {
						// Use the same realm and parent on all requests, inclusion tokens do not extend
						// to multiple parents sharing the same prefix or realms.
						r.WorkUnit.Realm = "testproject:otherrealm"
						r.Parent = parentWorkUnitID.Name()
					}

					t.Run("allowed with required permissions", func(t *ftt.Test) {
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.Loosely(t, err, should.BeNil)
					})
					t.Run("disallowed without create work unit permission", func(t *ftt.Test) {
						authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnit)
						_, err := recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[0]: caller does not have permission "resultdb.workUnits.create" in realm "testproject:otherrealm"`))
					})
					t.Run("disallowed with inclusion token for wrong realm", func(t *ftt.Test) {
						req.Requests[1].WorkUnit.Realm = "testproject:thirdrealm"

						_, err = recorder.BatchCreateWorkUnits(ctx, req)
						assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
						assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit: realm: got realm "testproject:thirdrealm" but inclusion token only authorizes the source to include realm "testproject:otherrealm"`))
					})
				})
			})
			t.Run("reserved id", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateWorkUnitWithReservedID)
				req.Requests[2].WorkUnitId = "build-8765432100"

				t.Run("disallowed without realm permission", func(t *ftt.Test) {
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit_id: only work units created by trusted systems may have id not starting with "u-"`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID,
					})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("producer resource", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permSetWorkUnitProducerResource)
				req.Requests[1].WorkUnit.ProducerResource = &pb.ProducerResource{
					System:    "buildbucket",
					DataRealm: "prod",
					Name:      "builds/1",
				}
				t.Run("disallowed without permission", func(t *ftt.Test) {
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`requests[1]: work_unit: producer_resource: only work units created by trusted system may have a populated producer_resource field`))
				})
				t.Run("allowed with realm permission", func(t *ftt.Test) {
					authState.IdentityPermissions = append(authState.IdentityPermissions, authtest.RealmPermission{
						Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource,
					})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with trusted group", func(t *ftt.Test) {
					authState.IdentityGroups = []string{trustedCreatorGroup}
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("allowed with non-validated system", func(t *ftt.Test) {
					req.Requests[1].WorkUnit.ProducerResource.System = "little-known-system"
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("inclusion or update token is validated", func(t *ftt.Test) {
				t.Run("invalid update token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})
				t.Run("invalid inclusion token", func(t *ftt.Test) {
					// Use the same parent on all requests as otherwise the request cannot be
					// authorized by an inclusion token (however valid it is).
					for _, r := range req.Requests {
						r.Parent = parentWorkUnitID.Name()
					}

					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.InclusionTokenMetadataKey, "invalid-token"))
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid inclusion token`))
				})
				t.Run("missing update and inclusion token", func(t *ftt.Test) {
					// Other test cases are covered by extractInclusionOrUpdateToken.
					ctx := metadata.NewOutgoingContext(ctx, metadata.MD{})
					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`expected either update-token or inclusion-token metadata value in the request`))
				})
			})
		})

		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": parentWorkUnitID.RootInvocationShardID(),
					"WorkUnitId":            parentWorkUnitID.WorkUnitID,
					"FinalizationState":     pb.WorkUnit_FINALIZING,
				}))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.That(t, err, should.ErrLike(`parent "rootInvocations/root-inv-id/workUnits/wu-parent" is not active`))
			})

			t.Run("parent does not exist and not created in the request", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", parentWorkUnitID.Key()))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/wu-parent" not found`))
			})
		})
		t.Run("already exists", func(t *ftt.Test) {
			t.Run("exist with different request id", func(t *ftt.Test) {
				// Create just one of the work units to be created.
				wu := workunits.NewBuilder(rootInvID, workUnitID11.WorkUnitID).WithCreateRequestID("different-request-id").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.That(t, err, should.ErrLike("\"rootInvocations/root-inv-id/workUnits/wu-parent:wu-new-11\" already exists with different requestID or creator"))
			})
			t.Run("partial exist with the same request id", func(t *ftt.Test) {
				// Create all work units to be created, but with a different request ID.
				wu := workunits.NewBuilder(rootInvID, workUnitID111.WorkUnitID).WithCreateRequestID("test-request-id").WithCreatedBy("user:someone@example.com").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				// The error message depends on which row is read first, so we just check for the general error string.
				assert.That(t, err, should.ErrLike("some work units already exist (eg. \"rootInvocations/root-inv-id/workUnits/wu-new-111\")"))
			})
		})

		t.Run("create is idempotent", func(t *ftt.Test) {
			res1, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Send the exact same request again.
			res2, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, res2, should.Match(res1))

			// Check the database has the expected number of entries.
			readCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			rows, err := workunits.ReadBatch(readCtx, []workunits.ID{workUnitID1, workUnitID11, workUnitID111}, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.HaveLength(3))
		})

		t.Run("requests attempts to prevent inheritance of module_ fields", func(t *ftt.Test) {
			t.Run("inherence chain starts from a stored work unit", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": parentWorkUnitID.RootInvocationShardID(),
					"WorkUnitId":            parentWorkUnitID.WorkUnitID,
					"ModuleName":            "mymodule",
					"ModuleScheme":          "gtest",
					"ModuleVariant":         pbutil.Variant("k", "v"),
					"ModuleShardKey":        "shard_key",
				}))

				t.Run("request overrides module_id", func(t *ftt.Test) {
					req.Requests[2].WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "other_module",
						ModuleScheme:  "junit",
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					req.Requests[2].WorkUnit.ModuleShardKey = "shard_key"

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit: module_id: must match module_id inherited from parent work unit with module set; got module name "other_module", was "mymodule"`))
				})
				t.Run("request overrides module_shard_key", func(t *ftt.Test) {
					req.Requests[2].WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "gtest",
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					req.Requests[2].WorkUnit.ModuleShardKey = ""

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit: module_shard_key: must match module_shard_key inherited from parent work unit with module set; got "", was "shard_key"`))
				})
			})
			t.Run("inheritance chain starts in a work unit created in the same request", func(t *ftt.Test) {
				req.Requests[0].WorkUnit.ModuleId = &pb.ModuleIdentifier{
					ModuleName:    "mymodule",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				}
				req.Requests[0].WorkUnit.ModuleShardKey = "shard_key"

				t.Run("request overrides module_id", func(t *ftt.Test) {
					req.Requests[2].WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "other_module",
						ModuleScheme:  "junit",
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					req.Requests[2].WorkUnit.ModuleShardKey = "shard_key"

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit: module_id: must match module_id inherited from parent work unit with module set; got module name "other_module", was "mymodule"`))
				})
				t.Run("request overrides module_shard_key", func(t *ftt.Test) {
					req.Requests[2].WorkUnit.ModuleId = &pb.ModuleIdentifier{
						ModuleName:    "mymodule",
						ModuleScheme:  "gtest",
						ModuleVariant: pbutil.Variant("k", "v"),
					}
					req.Requests[2].WorkUnit.ModuleShardKey = ""

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`requests[2]: work_unit: module_shard_key: must match module_shard_key inherited from parent work unit with module set; got "", was "shard_key"`))
				})
			})
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

			// Populare Requests[0] with all fields and Requests[1] and [2] with minimal fields
			// to maximise test coverage.
			req.Requests[0].WorkUnit = &pb.WorkUnit{
				Kind:            "EXAMPLE_MODULE",
				State:           pb.WorkUnit_RUNNING,
				Realm:           "testproject:testrealm",
				SummaryMarkdown: "Running FooBar...",
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:    "mymodule",
					ModuleScheme:  "gtest",
					ModuleVariant: pbutil.Variant("k", "v"),
				},
				ModuleShardKey: "shard_key",
				ProducerResource: &pb.ProducerResource{
					System:    "buildbucket",
					DataRealm: "prod",
					Name:      "builds/123",
				},
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				ExtendedProperties: extendedProperties,
				Instructions:       instructions,
			}

			// Expected work units in the response.
			expectedWU1 := proto.Clone(req.Requests[0].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU1, &pb.WorkUnit{
				Name:              workUnitID1.Name(),
				Parent:            parentWorkUnitID.Name(),
				WorkUnitId:        workUnitID1.WorkUnitID,
				FinalizationState: pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:           "user:someone@example.com",
				Deadline:          timestamppb.New(start.Add(defaultDeadlineDuration)),
				ChildWorkUnits:    []string{workUnitID11.Name()},
			})
			expectedWU1.Instructions = instructionutil.InstructionsWithNames(instructions, workUnitID1.Name())
			pbutil.PopulateModuleIdentifierHashes(expectedWU1.ModuleId)

			expectedWU11 := proto.Clone(req.Requests[1].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU11, &pb.WorkUnit{
				Name:              workUnitID11.Name(),
				Parent:            workUnitID1.Name(),
				WorkUnitId:        workUnitID11.WorkUnitID,
				FinalizationState: pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				State:             pb.WorkUnit_PENDING,
				Creator:           "user:someone@example.com",
				Deadline:          timestamppb.New(start.Add(defaultDeadlineDuration)),
				ModuleId:          expectedWU1.ModuleId,       // Inherited.
				ModuleShardKey:    expectedWU1.ModuleShardKey, // Inherited.
				ChildWorkUnits:    []string{workUnitID111.Name()},
			})

			expectedWU111 := proto.Clone(req.Requests[2].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU111, &pb.WorkUnit{
				Name:              workUnitID111.Name(),
				Parent:            workUnitID11.Name(),
				WorkUnitId:        workUnitID111.WorkUnitID,
				FinalizationState: pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				State:             pb.WorkUnit_PENDING,
				Creator:           "user:someone@example.com",
				Deadline:          timestamppb.New(start.Add(defaultDeadlineDuration)),
				ModuleId:          expectedWU1.ModuleId,       // Inherited.
				ModuleShardKey:    expectedWU1.ModuleShardKey, // Inherited.
			})

			expectWURow1 := &workunits.WorkUnitRow{
				ID:                workUnitID1,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				Kind:              "EXAMPLE_MODULE",
				State:             pb.WorkUnit_RUNNING,
				FinalizationState: pb.WorkUnit_ACTIVE,
				SummaryMarkdown:   "Running FooBar...",
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
				ModuleShardKey:          "shard_key",
				ModuleInheritanceStatus: workunits.ModuleInheritanceStatusRoot,
				ProducerResource: &pb.ProducerResource{
					System:    "buildbucket",
					DataRealm: "prod",
					Name:      "builds/123",
				},
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				Instructions:       instructionutil.InstructionsWithNames(instructions, workUnitID1.Name()),
				ExtendedProperties: extendedProperties,
				ChildWorkUnits:     []workunits.ID{workUnitID11},
			}

			expectWURow11 := &workunits.WorkUnitRow{
				ID:                      workUnitID11,
				ParentWorkUnitID:        spanner.NullString{Valid: true, StringVal: workUnitID1.WorkUnitID},
				Kind:                    "EXAMPLE_MODULE",
				State:                   pb.WorkUnit_PENDING,
				FinalizationState:       pb.WorkUnit_ACTIVE,
				Realm:                   "testproject:testrealm",
				CreatedBy:               "user:someone@example.com",
				FinalizeStartTime:       spanner.NullTime{},
				FinalizeTime:            spanner.NullTime{},
				Deadline:                start.Add(defaultDeadlineDuration),
				CreateRequestID:         "test-request-id",
				ModuleID:                expectWURow1.ModuleID,       // Inherited.
				ModuleShardKey:          expectWURow1.ModuleShardKey, // Inherited.
				ModuleInheritanceStatus: workunits.ModuleInheritanceStatusInherited,
				ChildWorkUnits:          []workunits.ID{workUnitID111},
			}

			expectWURow111 := &workunits.WorkUnitRow{
				ID:                      workUnitID111,
				ParentWorkUnitID:        spanner.NullString{Valid: true, StringVal: workUnitID11.WorkUnitID},
				Kind:                    "EXAMPLE_TEST_RUN",
				State:                   pb.WorkUnit_PENDING,
				FinalizationState:       pb.WorkUnit_ACTIVE,
				Realm:                   "testproject:testrealm",
				CreatedBy:               "user:someone@example.com",
				FinalizeStartTime:       spanner.NullTime{},
				FinalizeTime:            spanner.NullTime{},
				Deadline:                start.Add(defaultDeadlineDuration),
				CreateRequestID:         "test-request-id",
				ModuleID:                expectWURow1.ModuleID,       // Inherited.
				ModuleShardKey:          expectWURow1.ModuleShardKey, // Inherited.
				ModuleInheritanceStatus: workunits.ModuleInheritanceStatusInherited,
			}

			// Expected update token
			workUnitID11ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID11)
			assert.Loosely(t, err, should.BeNil)
			workUnitID111ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID111)
			assert.Loosely(t, err, should.BeNil)

			res, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			createTime := res.WorkUnits[0].CreateTime.AsTime()
			expectedWU1.CreateTime = timestamppb.New(createTime)
			expectedWU1.LastUpdated = timestamppb.New(createTime)
			expectedWU1.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			expectedWU11.CreateTime = timestamppb.New(createTime)
			expectedWU11.LastUpdated = timestamppb.New(createTime)
			expectedWU11.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			expectedWU111.CreateTime = timestamppb.New(createTime)
			expectedWU111.LastUpdated = timestamppb.New(createTime)
			expectedWU111.Etag = fmt.Sprintf(`W/"+f/%s"`, createTime.UTC().Format(time.RFC3339Nano))
			assert.That(t, res, should.Match(
				&pb.BatchCreateWorkUnitsResponse{
					WorkUnits: []*pb.WorkUnit{expectedWU1, expectedWU11, expectedWU111},
					UpdateTokens: []string{
						parentUpdateToken,
						workUnitID11ExpectedUpdateToken,
						workUnitID111ExpectedUpdateToken,
					},
				},
			))

			// Check the database.
			readCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			row, err := workunits.Read(readCtx, workUnitID1, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow1.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow1.CreateTime = createTime
			expectWURow1.LastUpdated = createTime
			assert.That(t, row, should.Match(expectWURow1))

			row, err = workunits.Read(readCtx, workUnitID11, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow11.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow11.CreateTime = createTime
			expectWURow11.LastUpdated = createTime
			expectWURow11.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow11))

			row, err = workunits.Read(readCtx, workUnitID111, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow111.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow111.CreateTime = createTime
			expectWURow111.LastUpdated = createTime
			expectWURow111.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow111))

			// Check the legacy invocation is inserted.
			legacyInvs, err := invocations.ReadBatch(readCtx, invocations.NewIDSet(workUnitID1.LegacyInvocationID(), workUnitID11.LegacyInvocationID(), workUnitID111.LegacyInvocationID()), invocations.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, legacyInvs, should.HaveLength(3))

			// Check inclusion is added to IncludedInvocations.
			includedIDs, err := invocations.ReadIncluded(readCtx, parentWorkUnitID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(1))
			assert.That(t, includedIDs.Has(workUnitID1.LegacyInvocationID()), should.BeTrue)

			includedIDs, err = invocations.ReadIncluded(readCtx, workUnitID1.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(1))
			assert.That(t, includedIDs.Has(workUnitID11.LegacyInvocationID()), should.BeTrue)

			includedIDs, err = invocations.ReadIncluded(readCtx, workUnitID11.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(1))
			assert.That(t, includedIDs.Has(workUnitID111.LegacyInvocationID()), should.BeTrue)
		})
	})
}

func TestPrepareWorkUnitCreationLogMessage(t *testing.T) {
	t.Parallel()

	ftt.Run("prepareWorkUnitCreationLogMessage", t, func(t *ftt.Test) {
		parentIDs := []workunits.ID{
			{RootInvocationID: "root-inv-1", WorkUnitID: "parent-wu-1"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "parent-wu-1"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "parent-wu-2"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "parent-wu-2"},
		}
		ids := []workunits.ID{
			{RootInvocationID: "root-inv-1", WorkUnitID: "child-wu-1"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "child-wu-2"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "child-wu-3"},
			{RootInvocationID: "root-inv-1", WorkUnitID: "child-wu-4"},
		}
		t.Run("single work unit", func(t *ftt.Test) {
			msg := prepareWorkUnitCreationLogMessage(parentIDs[:1], ids[:1])
			assert.Loosely(t, msg, should.Equal(`Creating work unit "child-wu-1" in "parent-wu-1" (root invocation: "root-inv-1")`))
		})
		t.Run("two work units", func(t *ftt.Test) {
			msg := prepareWorkUnitCreationLogMessage(parentIDs[:2], ids[:2])
			assert.Loosely(t, msg, should.Equal(`Creating work unit "child-wu-1" in "parent-wu-1" and "child-wu-2" in "parent-wu-1" (root invocation: "root-inv-1")`))
		})
		t.Run("three work units", func(t *ftt.Test) {
			msg := prepareWorkUnitCreationLogMessage(parentIDs[:3], ids[:3])
			assert.Loosely(t, msg, should.Equal(`Creating work unit "child-wu-1" in "parent-wu-1", "child-wu-2" in "parent-wu-1" and "child-wu-3" in "parent-wu-2" (root invocation: "root-inv-1")`))
		})
		t.Run("more than three work units", func(t *ftt.Test) {
			msg := prepareWorkUnitCreationLogMessage(parentIDs, ids)
			assert.Loosely(t, msg, should.Equal(`Creating work unit "child-wu-1" in "parent-wu-1", "child-wu-2" in "parent-wu-1" and "child-wu-3" in "parent-wu-2" (and 1 more) (root invocation: "root-inv-1")`))
		})
	})
}
