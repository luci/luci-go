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
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

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

func TestValidateBatchCreateWorkUnitsPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateBatchCreateWorkUnitsPermissions`, t, func(t *ftt.Test) {
		basePerms := []authtest.RealmPermission{
			{Realm: "project:realm", Permission: permCreateWorkUnit},
			{Realm: "project:realm", Permission: permIncludeWorkUnit},
		}

		authState := &authtest.FakeState{
			Identity:            "user:someone@example.com",
			IdentityPermissions: basePerms,
		}
		ctx := auth.WithState(context.Background(), authState)

		req := &pb.BatchCreateWorkUnitsRequest{
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu1",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu2",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
			},
			RequestId: "request-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no requests", func(t *ftt.Test) {
			req.Requests = nil
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})
		t.Run("too many requests", func(t *ftt.Test) {
			req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})

		t.Run("permission denied on one request", func(t *ftt.Test) {
			// We do not need to test all cases, as the verifyCreateWorkUnitPermissions
			// has its own tests.
			req.Requests[1].WorkUnit.Realm = "project:otherrealm"
			err := validateBatchCreateWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`requests[1]: caller does not have permission "resultdb.workUnits.create" in realm "project:otherrealm"`))
		})
	})
}

func TestValidateBatchCreateWorkUnitsRequest(t *testing.T) {
	t.Parallel()

	ftt.Run(`ValidateBatchCreateWorkUnitsRequest`, t, func(t *ftt.Test) {
		req := &pb.BatchCreateWorkUnitsRequest{
			Requests: []*pb.CreateWorkUnitRequest{
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu1",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
				{
					Parent:     "rootInvocations/u-my-root-id/workUnits/root",
					WorkUnitId: "u-wu2",
					WorkUnit: &pb.WorkUnit{
						Realm: "project:realm",
					},
				},
			},
			RequestId: "request-id",
		}

		t.Run("valid", func(t *ftt.Test) {
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("unspecified", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})

		t.Run("no requests", func(t *ftt.Test) {
			req.Requests = nil
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})

		t.Run("too many requests", func(t *ftt.Test) {
			req.Requests = make([]*pb.CreateWorkUnitRequest, 501)
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})
		t.Run("total size of requests is too large", func(t *ftt.Test) {
			req.Requests[0].Parent = strings.Repeat("a", pbutil.MaxBatchRequestSize)
			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests: the size of all requests is too large"))
		})

		t.Run("duplicated work unit id", func(t *ftt.Test) {
			req.Requests[1].WorkUnitId = req.Requests[0].WorkUnitId

			err := validateBatchCreateWorkUnitsRequest(req)
			assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit_id: duplicated work unit id"))
		})
		t.Run("sub-request", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				req.Requests[1].WorkUnitId = "invalid id"
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.Loosely(t, err, should.ErrLike("requests[1]: work_unit_id: does not match"))
			})
			t.Run("request_id", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req.Requests[0].RequestId = ""
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("inconsistent", func(t *ftt.Test) {
					req.Requests[0].RequestId = "another-id"
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.ErrLike("requests[0]: request_id: inconsistent with top-level request_id"))
				})
				t.Run("consistent", func(t *ftt.Test) {
					req.Requests[0].RequestId = req.RequestId
					err := validateBatchCreateWorkUnitsRequest(req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
		t.Run("ordering", func(t *ftt.Test) {
			t.Run("early entry refer to later entry", func(t *ftt.Test) {
				workUnitID3 := workunits.ID{
					RootInvocationID: "u-my-root-id",
					WorkUnitID:       "wu-new-3",
				}
				workUnitID4 := workunits.ID{
					RootInvocationID: "u-my-root-id",
					WorkUnitID:       "wu-new-4",
				}
				req.Requests = append(req.Requests,
					&pb.CreateWorkUnitRequest{
						Parent:     workUnitID4.Name(),
						WorkUnitId: workUnitID3.WorkUnitID,
						WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
					})
				req.Requests = append(req.Requests,
					&pb.CreateWorkUnitRequest{
						Parent:     "rootInvocations/u-my-root-id/workUnits/root",
						WorkUnitId: workUnitID4.WorkUnitID,
						WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
					})

				err := validateBatchCreateWorkUnitsRequest(req)
				assert.That(t, err, should.ErrLike("parent: cannot refer to work unit created in the later request"))
			})

			t.Run("request contains self loop wu3 <-> wu3", func(t *ftt.Test) {
				workUnitID3 := workunits.ID{
					RootInvocationID: "u-my-root-id",
					WorkUnitID:       "wu-parent:wu-new-3",
				}
				req.Requests = append(req.Requests,
					&pb.CreateWorkUnitRequest{
						Parent:     workUnitID3.Name(),
						WorkUnitId: workUnitID3.WorkUnitID,
						WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
					})
				err := validateBatchCreateWorkUnitsRequest(req)
				assert.That(t, err, should.ErrLike("cannot refer to work unit created in the later request"))
			})
		})
		t.Run("parent is a different root invocation id", func(t *ftt.Test) {
			req.Requests[1].Parent = "rootInvocations/u-my-root-id-2/workUnits/root"

			err := validateBatchCreateWorkUnitsRequest(req)
			assert.That(t, err, should.ErrLike("all requests must be for creations in the same root invocation"))
		})
	})
}

func TestBatchCreateWorkUnits(t *testing.T) {
	ftt.Run(`TestBatchCreateWorkUnits`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permCreateWorkUnit},
				{Realm: "testproject:testrealm", Permission: permIncludeWorkUnit},
				{Realm: "testproject:@root", Permission: permCreateWorkUnitWithReservedID},
				{Realm: "testproject:@root", Permission: permSetWorkUnitProducerResource},
			},
		})

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
		workUnitID1 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-parent:wu-new-1",
		}
		workUnitID11 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new-11",
		}
		workUnitID2 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "wu-new-2",
		}

		// Insert a root invocation and the parent work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
		parentWu := workunits.NewBuilder(rootInvID, parentWorkUnitID.WorkUnitID).WithState(pb.WorkUnit_ACTIVE).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(parentWu)...)

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
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
				},
				{
					Parent:     workUnitID1.Name(),
					WorkUnitId: workUnitID11.WorkUnitID,
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
				},
				{
					Parent:     parentWorkUnitID.Name(),
					WorkUnitId: workUnitID2.WorkUnitID,
					WorkUnit:   &pb.WorkUnit{Realm: "testproject:testrealm"},
				},
			},
		}

		t.Run("invalid request", func(t *ftt.Test) {
			// Full validation is tested in validateBatchCreateWorkUnitsRequest tests.
			// These tests just ensure that validation is being called.
			t.Run("missing top-level request id", func(t *ftt.Test) {
				req.RequestId = ""

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("request_id: unspecified"))
			})
		})

		t.Run("permission denied", func(t *ftt.Test) {
			// Permission checks are tested exhaustively in TestvalidateBatchCreateWorkUnitsPermissions.
			// This test just ensures they are called.
			t.Run("no create work unit permission", func(t *ftt.Test) {
				req.Requests[0].WorkUnit.Realm = "secretproject:testrealm"

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`requests[0]: caller does not have permission "resultdb.workUnits.create"`))
			})

			t.Run("update token", func(t *ftt.Test) {
				// Update token validation is not inside validateBatchCreateWorkUnitsPermissions, this need to be tested exhaustively here.
				t.Run("invalid update token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.That(t, err, should.ErrLike(`invalid update token`))
				})

				t.Run("missing update token", func(t *ftt.Test) {
					ctx := metadata.NewOutgoingContext(ctx, metadata.MD{})

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
					assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
				})

				t.Run("parent requires a different update token", func(t *ftt.Test) {
					anotherParent := workunits.ID{
						RootInvocationID: rootInvID,
						WorkUnitID:       "another-parent",
					}
					req.Requests[1].Parent = anotherParent.Name()

					_, err := recorder.BatchCreateWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike(`parent "rootInvocations/root-inv-id/workUnits/another-parent" requires a different update token`))
				})
			})
		})

		t.Run("parent", func(t *ftt.Test) {
			t.Run("parent not active", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanutil.UpdateMap("WorkUnits", map[string]any{
					"RootInvocationShardId": parentWorkUnitID.RootInvocationShardID(),
					"WorkUnitId":            parentWorkUnitID.WorkUnitID,
					"State":                 pb.WorkUnit_FINALIZING,
				}))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
				assert.That(t, err, should.ErrLike("parent \"rootInvocations/root-inv-id/workUnits/wu-parent\" is not active"))
			})

			t.Run("parent does not exist and not created in the request", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", parentWorkUnitID.Key()))

				_, err = recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.That(t, err, should.ErrLike("not found"))
			})
		})
		t.Run("already exists", func(t *ftt.Test) {
			t.Run("exist with different request id", func(t *ftt.Test) {
				// Create just one of the work units to be created.
				wu := workunits.NewBuilder(rootInvID, workUnitID11.WorkUnitID).WithCreateRequestID("different-request-id").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				assert.That(t, err, should.ErrLike("\"rootInvocations/root-inv-id/workUnits/wu-new-11\" already exists with different requestID or creator"))
			})
			t.Run("partial exist with the same request id", func(t *ftt.Test) {
				// Create all work units to be created, but with a different request ID.
				wu := workunits.NewBuilder(rootInvID, workUnitID2.WorkUnitID).WithCreateRequestID("test-request-id").WithCreatedBy("user:someone@example.com").Build()
				testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

				_, err := recorder.BatchCreateWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.AlreadyExists))
				// The error message depends on which row is read first, so we just check for the general error string.
				assert.That(t, err, should.ErrLike("some work units already exist (eg. \"rootInvocations/root-inv-id/workUnits/wu-new-2\")"))
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
			rows, err := workunits.ReadBatch(readCtx, []workunits.ID{workUnitID1, workUnitID11, workUnitID2}, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.HaveLength(3))
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

			// Fill all fields to enhance test coverage.
			req.Requests[0].WorkUnit = &pb.WorkUnit{
				Realm:              "testproject:testrealm",
				State:              pb.WorkUnit_FINALIZING,
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				ExtendedProperties: extendedProperties,
				Instructions:       instructions,
			}

			// Expected work units in the response.
			expectedWU1 := proto.Clone(req.Requests[0].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU1, &pb.WorkUnit{
				Name:           workUnitID1.Name(),
				Parent:         parentWorkUnitID.Name(),
				WorkUnitId:     workUnitID1.WorkUnitID,
				Creator:        "user:someone@example.com",
				Deadline:       timestamppb.New(start.Add(defaultDeadlineDuration)),
				ChildWorkUnits: []string{workUnitID11.Name()},
			})
			expectedWU1.Instructions = instructionutil.InstructionsWithNames(instructions, workUnitID1.Name())

			expectedWU11 := proto.Clone(req.Requests[1].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU11, &pb.WorkUnit{
				Name:       workUnitID11.Name(),
				Parent:     workUnitID1.Name(),
				WorkUnitId: workUnitID11.WorkUnitID,
				State:      pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:    "user:someone@example.com",
				Deadline:   timestamppb.New(start.Add(defaultDeadlineDuration)),
			})

			expectedWU2 := proto.Clone(req.Requests[2].WorkUnit).(*pb.WorkUnit)
			proto.Merge(expectedWU2, &pb.WorkUnit{
				Name:       workUnitID2.Name(),
				Parent:     parentWorkUnitID.Name(),
				WorkUnitId: workUnitID2.WorkUnitID,
				State:      pb.WorkUnit_ACTIVE, // Default state is ACTIVE.
				Creator:    "user:someone@example.com",
				Deadline:   timestamppb.New(start.Add(defaultDeadlineDuration)),
			})

			expectWURow1 := &workunits.WorkUnitRow{
				ID:                 workUnitID1,
				ParentWorkUnitID:   spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				State:              pb.WorkUnit_FINALIZING,
				Realm:              "testproject:testrealm",
				CreatedBy:          "user:someone@example.com",
				FinalizeStartTime:  spanner.NullTime{},
				FinalizeTime:       spanner.NullTime{},
				Deadline:           start.Add(defaultDeadlineDuration),
				CreateRequestID:    "test-request-id",
				ProducerResource:   "//producer.example.com/builds/123",
				Tags:               pbutil.StringPairs("e2e_key", "e2e_value"),
				Properties:         wuProperties,
				Instructions:       instructionutil.InstructionsWithNames(instructions, workUnitID1.Name()),
				ExtendedProperties: extendedProperties,
				ChildWorkUnits:     []workunits.ID{workUnitID11},
			}

			expectWURow11 := &workunits.WorkUnitRow{
				ID:                workUnitID11,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: workUnitID1.WorkUnitID},
				State:             pb.WorkUnit_ACTIVE,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
			}

			expectWURow2 := &workunits.WorkUnitRow{
				ID:                workUnitID2,
				ParentWorkUnitID:  spanner.NullString{Valid: true, StringVal: parentWorkUnitID.WorkUnitID},
				State:             pb.WorkUnit_ACTIVE,
				Realm:             "testproject:testrealm",
				CreatedBy:         "user:someone@example.com",
				FinalizeStartTime: spanner.NullTime{},
				FinalizeTime:      spanner.NullTime{},
				Deadline:          start.Add(defaultDeadlineDuration),
				CreateRequestID:   "test-request-id",
			}

			// Expected update token
			workUnitID11ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID11)
			assert.Loosely(t, err, should.BeNil)
			workUnitID2ExpectedUpdateToken, err := generateWorkUnitUpdateToken(ctx, workUnitID2)
			assert.Loosely(t, err, should.BeNil)

			res, err := recorder.BatchCreateWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			expectedWU1.CreateTime = res.WorkUnits[0].CreateTime
			expectedWU1.FinalizeStartTime = res.WorkUnits[0].FinalizeStartTime
			expectedWU11.CreateTime = res.WorkUnits[1].CreateTime
			expectedWU2.CreateTime = res.WorkUnits[2].CreateTime
			assert.That(t, res, should.Match(
				&pb.BatchCreateWorkUnitsResponse{
					WorkUnits: []*pb.WorkUnit{expectedWU1, expectedWU11, expectedWU2},
					UpdateTokens: []string{
						parentUpdateToken,
						workUnitID11ExpectedUpdateToken,
						workUnitID2ExpectedUpdateToken,
					},
				},
			))
			assert.Loosely(t, res.WorkUnits[0].FinalizeStartTime, should.NotBeNil)

			// Check the database.
			readCtx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			row, err := workunits.Read(readCtx, workUnitID1, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow1.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow1.CreateTime = row.CreateTime
			expectWURow1.FinalizeStartTime = row.FinalizeStartTime
			assert.That(t, row, should.Match(expectWURow1))

			row, err = workunits.Read(readCtx, workUnitID2, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow2.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow2.CreateTime = row.CreateTime
			expectWURow2.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow2))

			row, err = workunits.Read(readCtx, workUnitID11, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			expectWURow11.SecondaryIndexShardID = row.SecondaryIndexShardID
			expectWURow11.CreateTime = row.CreateTime
			expectWURow11.Tags = []*pb.StringPair{}
			assert.That(t, row, should.Match(expectWURow11))

			// Check the legacy invocation is inserted.
			legacyInvs, err := invocations.ReadBatch(readCtx, invocations.NewIDSet(workUnitID1.LegacyInvocationID(), workUnitID11.LegacyInvocationID(), workUnitID2.LegacyInvocationID()), invocations.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, legacyInvs, should.HaveLength(3))

			// Check inclusion is added to IncludedInvocations.
			includedIDs, err := invocations.ReadIncluded(readCtx, parentWorkUnitID.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(2))
			assert.That(t, includedIDs.Has(workUnitID1.LegacyInvocationID()), should.BeTrue)
			assert.That(t, includedIDs.Has(workUnitID2.LegacyInvocationID()), should.BeTrue)

			includedIDs, err = invocations.ReadIncluded(readCtx, workUnitID1.LegacyInvocationID())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, includedIDs, should.HaveLength(1))
			assert.That(t, includedIDs.Has(workUnitID11.LegacyInvocationID()), should.BeTrue)
		})
	})
}
