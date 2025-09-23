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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateUpdateRootInvocationRequest(t *testing.T) {
	t.Parallel()

	ftt.Run("TestValidateUpdateRootInvocationRequest", t, func(t *ftt.Test) {
		ctx := context.Background()
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		req := &pb.UpdateRootInvocationRequest{
			RootInvocation: &pb.RootInvocation{
				Name: "rootInvocations/inv",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"tags"}},
			RequestId:  "test-request-id",
		}

		t.Run("etag", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				// Empty is valid.
				req.RootInvocation.Etag = ""

				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocation.Etag = "invalid"

				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.That(t, err, should.ErrLike(`root_invocation: etag: malformated etag`))
			})
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RequestId = ""
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})

		t.Run("empty update mask", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{}

			err := validateUpdateRootInvocationRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike("update_mask: paths is empty"))
		})

		t.Run("non-exist update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"not_exist"}
			err := validateUpdateRootInvocationRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`update_mask: field "not_exist" does not exist in message RootInvocation`))
		})

		t.Run("unsupported update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"name"}
			err := validateUpdateRootInvocationRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`update_mask: unsupported path "name"`))
		})

		t.Run("submask in update mask", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"deadline.seconds"}
			err := validateUpdateRootInvocationRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`update_mask: "deadline" should not have any submask`))
		})

		t.Run("state", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"state"}

			t.Run("valid FINALIZING", func(t *ftt.Test) {
				req.RootInvocation.State = pb.RootInvocation_FINALIZING
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("valid ACTIVE", func(t *ftt.Test) {
				req.RootInvocation.State = pb.RootInvocation_ACTIVE
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid FINALIZED", func(t *ftt.Test) {
				req.RootInvocation.State = pb.RootInvocation_FINALIZED
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike("root_invocation: state: must be FINALIZING or ACTIVE"))
			})
		})

		t.Run("deadline", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"deadline"}
			t.Run("valid", func(t *ftt.Test) {
				req.RootInvocation.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid past", func(t *ftt.Test) {
				req.RootInvocation.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`root_invocation: deadline: must be at least 10 seconds in the future`))
			})
		})

		t.Run("sources", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"sources"}
			t.Run("valid", func(t *ftt.Test) {
				req.RootInvocation.Sources = testutil.TestSources()
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocation.Sources = &pb.Sources{
					BaseSources: &pb.Sources_GitilesCommit{
						GitilesCommit: &pb.GitilesCommit{Host: "invalid host"},
					},
				}
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike("root_invocation: sources: gitiles_commit: host: does not match"))
			})
		})

		t.Run("sources_final", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"sources_final"}
			req.RootInvocation.SourcesFinal = true
			err := validateUpdateRootInvocationRequest(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("tags", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"tags"}
			t.Run("valid", func(t *ftt.Test) {
				req.RootInvocation.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocation.Tags = []*pb.StringPair{{Key: "k", Value: "a\n"}}
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`root_invocation: tags: "k":"a\n": value: non-printable rune '\n' at byte index 1`))
			})
		})

		t.Run("properties", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"properties"}
			t.Run("valid", func(t *ftt.Test) {
				req.RootInvocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type": structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"key_1": structpb.NewStringValue("value_1"),
					},
				}
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocation.Properties = &structpb.Struct{Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("1"),
				}}
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`root_invocation: properties: must have a field "@type"`))
			})
		})

		t.Run("baseline_id", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"baseline_id"}
			t.Run("valid", func(t *ftt.Test) {
				req.RootInvocation.BaselineId = "try:linux-rel"
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.RootInvocation.BaselineId = "invalid-baseline"
				err := validateUpdateRootInvocationRequest(ctx, req)
				assert.Loosely(t, err, should.ErrLike("root_invocation: baseline_id: does not match"))
			})
		})
	})
}

func TestUpdateRootInvocation(t *testing.T) {
	ftt.Run("TestUpdateRootInvocation", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		recorder := newTestRecorderServer()
		rootInvID := rootinvocations.ID("rootid")
		ctx, sched := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)
		user := "user:someone@example.com"
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity(user),
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:@project", Permission: permPutBaseline},
			},
		})

		// A simple valid request.
		req := &pb.UpdateRootInvocationRequest{
			RootInvocation: &pb.RootInvocation{Name: rootInvID.Name(), State: pb.RootInvocation_FINALIZING},
			UpdateMask:     &field_mask.FieldMask{Paths: []string{"state"}},
			RequestId:      "test-request-id",
		}

		// Insert root invocation.
		expectedRootInvRow := rootinvocations.NewBuilder(rootInvID).
			WithState(pb.RootInvocation_ACTIVE).
			WithIsSourcesFinal(false).
			Build()
		testutil.MustApply(ctx, t, insert.RootInvocationOnly(expectedRootInvRow)...)
		expectedRootInv := expectedRootInvRow.ToProto()

		// Attach a valid update token for the root work unit.
		rootWorkUnitID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: workunits.RootWorkUnitID}
		token, err := generateWorkUnitUpdateToken(ctx, rootWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("request validate", func(t *ftt.Test) {
			t.Run("unspecified root invocation", func(t *ftt.Test) {
				req.RootInvocation = nil
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: root_invocation: unspecified"))
			})

			t.Run("invalid name", func(t *ftt.Test) {
				req.RootInvocation.Name = "invalid"
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: root_invocation: name: does not match pattern"))
			})

			// validateUpdateRootInvocationRequest has its own exhaustive test cases,
			// simply check that it is called.
			t.Run("other invalid", func(t *ftt.Test) {
				req.RootInvocation.State = pb.RootInvocation_FINALIZED
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: root_invocation: state: must be FINALIZING or ACTIVE"))
			})
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`missing update-token metadata value in the request`))
			})

			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`invalid update token`))
			})

			t.Run("baseline_id permission", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"baseline_id"}
				t.Run("denied", func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:user@example.com",
					})
					_, err := recorder.UpdateRootInvocation(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike(`caller does not have permission to write to test baseline in realm testproject:@project`))
				})

				t.Run("granted", func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:baseliner@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							{Realm: "testproject:@project", Permission: permPutBaseline},
						},
					})
					_, err := recorder.UpdateRootInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})

		t.Run("no root invocation", func(t *ftt.Test) {
			nonexistRootInvocationID := rootinvocations.ID("nonexist")
			req.RootInvocation.Name = nonexistRootInvocationID.Name()
			token, err := generateWorkUnitUpdateToken(ctx, workunits.ID{RootInvocationID: nonexistRootInvocationID, WorkUnitID: workunits.RootWorkUnitID})
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))
			_, err = recorder.UpdateRootInvocation(ctx, req)

			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`"rootInvocations/nonexist" not found`))
		})

		t.Run("update is idempotent", func(t *ftt.Test) {
			t.Run("deduplicated with request_id", func(t *ftt.Test) {
				req.RootInvocation.Tags = []*pb.StringPair{{Key: "updatedkey", Value: "updatedval"}}
				req.UpdateMask.Paths = []string{"tags"}
				req.RootInvocation.Etag = expectedRootInv.Etag
				res, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Send the exact same request again, the etag is not updated so update
				// should fail by etag mismatch if it is not deduplicated.
				res2, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res2, should.Match(res))
			})
		})

		t.Run("root invocation not active", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"state"}
			req.RootInvocation.State = pb.RootInvocation_FINALIZING
			_, err := recorder.UpdateRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Use a new request id to avoid the repeated request being deduplicated.
			req.RequestId = "new-request-id"
			_, err = recorder.UpdateRootInvocation(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike(`root invocation "rootInvocations/rootid" is not active`))
		})

		t.Run("etag", func(t *ftt.Test) {
			t.Run("unmatched etag", func(t *ftt.Test) {
				// Root invocation updated.
				req.UpdateMask.Paths = []string{"tags"}
				req.RootInvocation.Tags = []*pb.StringPair{{Key: "nk", Value: "nv"}}
				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Use a new request id to avoid the repeated request being deduplicated.
				// Sent a request with the old etag.
				req.RequestId = "new-request-id"
				req.RootInvocation.Etag = expectedRootInv.Etag
				_, err = recorder.UpdateRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Aborted))
				assert.That(t, err, should.ErrLike(`the root invocation was modified since it was last read; the update was not applied`))
			})

			t.Run("match etag", func(t *ftt.Test) {
				req.RootInvocation.Etag = rootinvocations.Etag(expectedRootInvRow)

				_, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run("e2e", func(t *ftt.Test) {
			assertResponse := func(rootInv *pb.RootInvocation, expected *pb.RootInvocation) {
				// Etag is must be different if updated.
				assert.That(t, rootInv.Etag, should.NotEqual(expected.Etag), truth.LineContext())
				assert.Loosely(t, rootInv.Etag, should.NotBeEmpty, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, rootInv.LastUpdated.AsTime(), should.HappenAfter(expected.LastUpdated.AsTime()), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				if expected.State != pb.RootInvocation_ACTIVE {
					assert.Loosely(t, rootInv.FinalizeStartTime, should.Match(rootInv.LastUpdated), truth.LineContext())
				} else {
					assert.Loosely(t, rootInv.FinalizeStartTime, should.BeNil, truth.LineContext())
				}
				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expectedCopy := proto.Clone(expected).(*pb.RootInvocation)
				expectedCopy.LastUpdated = rootInv.LastUpdated
				expectedCopy.Etag = rootInv.Etag
				expectedCopy.FinalizeStartTime = rootInv.FinalizeStartTime
				assert.That(t, rootInv, should.Match(expectedCopy), truth.LineContext())
			}

			assertSpannerRows := func(expectedRow *rootinvocations.RootInvocationRow) {
				rootInvRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, rootInvRow.LastUpdated, should.HappenAfter(expectedRow.LastUpdated), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				shouldSetfinalizeStartTime := expectedRow.State != pb.RootInvocation_ACTIVE
				assert.Loosely(t, rootInvRow.FinalizeStartTime.Valid, should.Equal(shouldSetfinalizeStartTime), truth.LineContext())
				if shouldSetfinalizeStartTime {
					assert.Loosely(t, rootInvRow.FinalizeStartTime.Time, should.Match(rootInvRow.LastUpdated), truth.LineContext())
				}

				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expectedRowCopy := expectedRow.Clone()
				expectedRowCopy.LastUpdated = rootInvRow.LastUpdated
				expectedRowCopy.FinalizeStartTime = rootInvRow.FinalizeStartTime
				// Validate RootInvocations table.
				assert.That(t, rootInvRow, should.Match(expectedRowCopy), truth.LineContext())

				// Validate RootInvocationShards table.
				for shardID := range rootInvID.AllShardIDs() {
					var compressedSources spanutil.Compressed
					var shardState pb.RootInvocation_State
					var shardRealm string
					var shardIsSourcesFinal bool
					err := spanutil.ReadRow(span.Single(ctx), "RootInvocationShards", shardID.Key(), map[string]any{
						"State":          &shardState,
						"Realm":          &shardRealm,
						"Sources":        &compressedSources,
						"IsSourcesFinal": &shardIsSourcesFinal,
					})
					assert.Loosely(t, err, should.BeNil)
					shardSources := &pb.Sources{}
					assert.Loosely(t, proto.Unmarshal(compressedSources, shardSources), should.BeNil)
					assert.Loosely(t, shardState, should.Equal(expectedRowCopy.State), truth.LineContext())
					assert.Loosely(t, shardRealm, should.Equal(expectedRowCopy.Realm), truth.LineContext())
					assert.Loosely(t, shardSources, should.Match(expectedRowCopy.Sources), truth.LineContext())
					assert.Loosely(t, shardIsSourcesFinal, should.Equal(expectedRowCopy.IsSourcesFinal), truth.LineContext())
				}

				// Validate legacy Invocations table.
				inv, err := invocations.Read(span.Single(ctx), rootInvID.LegacyInvocationID(), invocations.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv, should.Match(expectedRowCopy.ToLegacyInvocationProto()))
				// The root invocation update request should be recorded.
				exist, err := rootinvocations.CheckRootInvocationUpdateRequestExist(span.Single(ctx), rootInvID, user, "test-request-id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exist, should.BeTrue)
			}

			assertNoOp := func(respRootInv *pb.RootInvocation, expectedRow *rootinvocations.RootInvocationRow, expected *pb.RootInvocation) {
				// Assert response.
				assert.That(t, respRootInv, should.Match(expected), truth.LineContext())
				// Assert spanner.
				rootInvRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				assert.That(t, rootInvRow, should.Match(expectedRow), truth.LineContext())

				// The root invocation update request should be recorded even for no-pp.
				exist, err := rootinvocations.CheckRootInvocationUpdateRequestExist(span.Single(ctx), rootInvID, user, "test-request-id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exist, should.BeTrue)
			}

			t.Run("state", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"state"}
				req.RootInvocation.State = pb.RootInvocation_FINALIZING

				ri, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedRootInv.State = pb.RootInvocation_FINALIZING
				assertResponse(ri, expectedRootInv)

				// Validate spanner records are updated.
				expectedRootInvRow.State = pb.RootInvocation_FINALIZING
				assertSpannerRows(expectedRootInvRow)

				// Enqueued the finalization task.
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match([]protoreflect.ProtoMessage{
					&taskspb.RunExportNotifications{InvocationId: string(rootInvID.LegacyInvocationID())},
					&taskspb.TryFinalizeInvocation{InvocationId: string(rootInvID.LegacyInvocationID())},
				}))
			})

			t.Run("sources and sources_final", func(t *ftt.Test) {
				newSources := testutil.TestSourcesWithChangelistNumbers(123456)

				t.Run("source not finalized", func(t *ftt.Test) {
					t.Run("update sources", func(t *ftt.Test) {
						req.UpdateMask.Paths = []string{"sources"}
						req.RootInvocation.Sources = newSources

						ri, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						expectedRootInv.Sources = newSources
						assertResponse(ri, expectedRootInv)

						// Validate spanner records are updated.
						expectedRootInvRow.Sources = newSources
						assertSpannerRows(expectedRootInvRow)
					})

					t.Run("to non-finalized sources", func(t *ftt.Test) {
						// Should be no-op.
						req.UpdateMask.Paths = []string{"sources_final"}
						req.RootInvocation.SourcesFinal = false

						ri, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assertNoOp(ri, expectedRootInvRow, expectedRootInv)
					})

					t.Run("to finalized sources", func(t *ftt.Test) {
						req.UpdateMask.Paths = []string{"sources_final"}
						req.RootInvocation.SourcesFinal = true

						ri, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						expectedRootInv.SourcesFinal = true
						assertResponse(ri, expectedRootInv)

						// Validate spanner records are updated.
						expectedRootInvRow.IsSourcesFinal = true
						assertSpannerRows(expectedRootInvRow)
					})
				})

				t.Run("source finalized", func(t *ftt.Test) {
					// Update sources_final to true.
					req.UpdateMask.Paths = []string{"sources_final"}
					req.RootInvocation.SourcesFinal = true
					ri, err := recorder.UpdateRootInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)

					expectedRootInv.Etag = ri.Etag
					expectedRootInv.LastUpdated = ri.LastUpdated
					expectedRootInv.SourcesFinal = true
					expectedRootInvRow.LastUpdated = ri.LastUpdated.AsTime()
					expectedRootInvRow.IsSourcesFinal = true
					// Use a new request id to avoid the repeated request being deduplicated.
					req.RequestId = "new-request-id"

					t.Run("fail to update sources", func(t *ftt.Test) {
						req.UpdateMask.Paths = []string{"sources"}
						req.RootInvocation.Sources = newSources

						_, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("root_invocation: sources: cannot modify already finalized sources"))
					})

					t.Run("to finalized sources", func(t *ftt.Test) {
						// Should be no-op.
						req.UpdateMask.Paths = []string{"sources_final"}
						req.RootInvocation.SourcesFinal = true

						ri, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assertNoOp(ri, expectedRootInvRow, expectedRootInv)
					})

					t.Run("to non-finalized sources", func(t *ftt.Test) {
						req.UpdateMask.Paths = []string{"sources_final"}
						req.RootInvocation.SourcesFinal = false

						_, err := recorder.UpdateRootInvocation(ctx, req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("root_invocation: sources_final: cannot un-finalize already finalized sources"))
					})
				})
			})

			t.Run("deadline, properties, tags, baseline_id", func(t *ftt.Test) {
				newDeadline := pbutil.MustTimestampProto(now.Add(3 * time.Hour))
				newProperties := testutil.TestStrictProperties()
				newTags := []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
				newBaselineID := "try:new-baseline"

				req.UpdateMask.Paths = []string{"deadline", "properties", "tags", "baseline_id"}
				req.RootInvocation = &pb.RootInvocation{
					Name:       rootInvID.Name(),
					Deadline:   newDeadline,
					Properties: newProperties,
					Tags:       newTags,
					BaselineId: newBaselineID,
				}

				ri, err := recorder.UpdateRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedRootInv.BaselineId = newBaselineID
				expectedRootInv.Properties = newProperties
				expectedRootInv.Tags = newTags
				expectedRootInv.Deadline = newDeadline
				assertResponse(ri, expectedRootInv)

				// Validate spanner records are updated.
				expectedRootInvRow.BaselineID = newBaselineID
				expectedRootInvRow.Properties = newProperties
				expectedRootInvRow.Tags = newTags
				expectedRootInvRow.Deadline = newDeadline.AsTime()
				assertSpannerRows(expectedRootInvRow)
			})
		})
	})
}
