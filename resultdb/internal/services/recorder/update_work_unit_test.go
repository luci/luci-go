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

		// A valid base request.
		req := &pb.UpdateWorkUnitRequest{
			WorkUnit: &pb.WorkUnit{
				Name: "rootInvocations/inv/workUnits/wu",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"tags"}},
			RequestId:  "test-request-id",
		}
		requireRequestID := true
		t.Run("etag", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				// Empty is valid.
				req.WorkUnit.Etag = ""

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.Etag = "invalid"

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.That(t, err, should.ErrLike(`work_unit: etag: malformated etag`))
			})
		})
		t.Run("empty update mask", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{}
			err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
			assert.Loosely(t, err, should.ErrLike("update_mask: paths is empty"))
		})

		t.Run("non-exist update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"not_exist"}
			err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
			assert.Loosely(t, err, should.ErrLike(`update_mask: field "not_exist" does not exist in message WorkUnit`))
		})

		t.Run("unsupported update mask path", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"name"}
			err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
			assert.Loosely(t, err, should.ErrLike(`update_mask: unsupported path "name"`))
		})

		t.Run("request_id", func(t *ftt.Test) {
			t.Run("empty", func(t *ftt.Test) {
				req.RequestId = ""
				t.Run("required", func(t *ftt.Test) {
					requireRequestID = true
					err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
					assert.Loosely(t, err, should.ErrLike("request_id: unspecified"))
				})
				t.Run("not required", func(t *ftt.Test) {
					requireRequestID = false
					err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.RequestId = "😃"
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
			})
		})

		t.Run("submask in update mask", func(t *ftt.Test) {
			t.Run("unsupported", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"deadline.seconds"}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`update_mask: "deadline" should not have any submask`))
			})

			t.Run("supported for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.some_key"}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid key for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.invalid_key_"}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties: key "invalid_key_": does not match`))
			})

			t.Run("too deep for extended_properties", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"extended_properties.some_key.fields"}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties["some_key"] should not have any submask`))
			})
		})

		t.Run("state", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"state"}
				states := []pb.WorkUnit_State{pb.WorkUnit_PENDING, pb.WorkUnit_RUNNING, pb.WorkUnit_FAILED, pb.WorkUnit_CANCELLED, pb.WorkUnit_SKIPPED, pb.WorkUnit_SUCCEEDED}
				for _, state := range states {
					req.WorkUnit.State = state
					err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
					assert.Loosely(t, err, should.BeNil)
				}
			})
			t.Run("unspecified", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_STATE_UNSPECIFIED
				req.UpdateMask.Paths = []string{"state"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit: state: unspecified"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.State = pb.WorkUnit_FINAL_STATE_MASK
				req.UpdateMask.Paths = []string{"state"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit: state: FINAL_STATE_MASK is not a valid state"))
			})
		})

		t.Run("summary_markdown", func(t *ftt.Test) {
			t.Run("valid empty", func(t *ftt.Test) {
				req.WorkUnit.SummaryMarkdown = ""
				req.UpdateMask.Paths = []string{"summary_markdown"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("valid non-empty", func(t *ftt.Test) {
				req.WorkUnit.SummaryMarkdown = "The task failed because of..."
				req.UpdateMask.Paths = []string{"summary_markdown"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				// Not valid UTF-8.
				req.WorkUnit.SummaryMarkdown = "\xFF"
				req.UpdateMask.Paths = []string{"summary_markdown"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit: summary_markdown: not valid UTF-8"))
			})
			t.Run("too long", func(t *ftt.Test) {
				req.WorkUnit.SummaryMarkdown = strings.Repeat("a", 4097)
				req.UpdateMask.Paths = []string{"summary_markdown"}

				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike("work_unit: summary_markdown: must be at most 4096 bytes long (was 4097 bytes)"))
			})
		})

		t.Run("deadline", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"deadline"}
			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(time.Hour))
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid empty", func(t *ftt.Test) {
				req.WorkUnit.Deadline = nil
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`invalid nil Timestamp`))
			})

			t.Run("invalid past", func(t *ftt.Test) {
				req.WorkUnit.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`work_unit: deadline: must be at least 10 seconds in the future`))
			})
		})

		t.Run("tags", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"tags"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "k", Value: "a\n"}}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
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
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.Properties = &structpb.Struct{Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("1"),
				}}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`work_unit: properties: must have a field "@type"`))
			})
		})

		t.Run("extended_properties", func(t *ftt.Test) {
			req.UpdateMask.Paths = []string{"extended_properties"}

			t.Run("valid", func(t *ftt.Test) {
				req.WorkUnit.ExtendedProperties = testutil.TestInvocationExtendedProperties()
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid", func(t *ftt.Test) {
				req.WorkUnit.ExtendedProperties = map[string]*structpb.Struct{
					"key": {Fields: map[string]*structpb.Value{
						"a": structpb.NewStringValue("1"),
					}},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
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
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("invalid duplicate id", func(t *ftt.Test) {
				req.WorkUnit.Instructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{Id: "dup-id", Type: pb.InstructionType_STEP_INSTRUCTION, DescriptiveName: "des_name"},
						{Id: "dup-id", Type: pb.InstructionType_STEP_INSTRUCTION, DescriptiveName: "des_name"},
					},
				}
				err := validateUpdateWorkUnitRequest(ctx, req, requireRequestID)
				assert.Loosely(t, err, should.ErrLike(`work_unit: instructions: instructions[1]: id: "dup-id" is re-used at index 0`))
			})
		})
	})
}

func TestUpdateWorkUnit(t *testing.T) {
	ftt.Run("TestUpdateWorkUnit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		user := "user:someone@example.com"
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: identity.Identity(user),
		})
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

		// Attach a valid update token.
		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		// Insert root invocation and work unit into spanner.
		expectedWURow := workunits.
			NewBuilder(wuID.RootInvocationID, wuID.WorkUnitID).
			WithState(pb.WorkUnit_RUNNING).
			WithFinalizationState(pb.WorkUnit_ACTIVE).
			Build()

		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder(rootInvID).Build())...)
		ms = append(ms, insert.WorkUnit(expectedWURow)...)
		testutil.MustApply(ctx, t, ms...)

		expectedWU := masking.WorkUnit(expectedWURow, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)

		req := &pb.UpdateWorkUnitRequest{
			WorkUnit: &pb.WorkUnit{
				Name:  wuID.Name(),
				State: pb.WorkUnit_RUNNING,
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
			RequestId:  "test-request-id",
		}

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
				req.WorkUnit.State = pb.WorkUnit_FINAL_STATE_MASK
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("bad request: work_unit: state: FINAL_STATE_MASK is not a valid state"))
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

		t.Run("update is idempotent", func(t *ftt.Test) {
			t.Run("deduplicated with request_id", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"tags"}
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "nk", Value: "nv"}}
				req.WorkUnit.Etag = expectedWU.Etag
				res, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Send the exact same request again, the etag is not updated so update
				// should fail by etag mismatch if it is not deduplicated.
				res2, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, res2, should.Match(res))
			})
		})
		t.Run("etag", func(t *ftt.Test) {
			t.Run("unmatch etag", func(t *ftt.Test) {
				// Work unit updated.
				req.UpdateMask.Paths = []string{"tags"}
				req.WorkUnit.Tags = []*pb.StringPair{{Key: "nk", Value: "nv"}}
				_, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Request sent with the old etag, and a new request id to avoid request been deduplicated.
				req.RequestId = "new-request-id"
				req.WorkUnit.Etag = expectedWU.Etag
				_, err = recorder.UpdateWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Aborted))
				assert.That(t, err, should.ErrLike(`desc = the work unit was modified since it was last read; the update was not applied`))
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
			assert.Loosely(t, err, should.ErrLike(`desc = "rootInvocations/rootid/workUnits/nonexist" not found`))
		})

		t.Run("work unit not active", func(t *ftt.Test) {
			// Finalize work unit first.
			req.UpdateMask.Paths = []string{"state"}
			req.WorkUnit.State = pb.WorkUnit_SUCCEEDED

			_, err := recorder.UpdateWorkUnit(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			// Use a new request id to avoid request been deduplicated.
			req.RequestId = "new-request-id"
			_, err = recorder.UpdateWorkUnit(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike(`desc = work unit "rootInvocations/rootid/workUnits/wu" is not active`))
		})

		t.Run("e2e", func(t *ftt.Test) {
			assertResponse := func(wu *pb.WorkUnit, expected *pb.WorkUnit) {
				t.Helper()
				// Etag is must be different if updated.
				assert.That(t, wu.Etag, should.NotEqual(expected.Etag), truth.LineContext())
				assert.Loosely(t, wu.Etag, should.NotBeEmpty, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, wu.LastUpdated.AsTime(), should.HappenAfter(expected.LastUpdated.AsTime()), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				if expected.FinalizationState != pb.WorkUnit_ACTIVE {
					assert.Loosely(t, wu.FinalizeStartTime, should.Match(wu.LastUpdated), truth.LineContext())
				} else {
					assert.Loosely(t, wu.FinalizeStartTime, should.BeNil, truth.LineContext())
				}
				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expectedCopy := proto.Clone(expected).(*pb.WorkUnit)
				expectedCopy.LastUpdated = wu.LastUpdated
				expectedCopy.Etag = wu.Etag
				expectedCopy.FinalizeStartTime = wu.FinalizeStartTime
				assert.That(t, wu, should.Match(expectedCopy), truth.LineContext())
			}

			assertSpannerRows := func(expectedRow *workunits.WorkUnitRow) {
				t.Helper()
				wuID := expectedRow.ID
				wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				// LastUpdated time must move forward.
				assert.That(t, wuRow.LastUpdated, should.HappenAfter(expectedRow.LastUpdated), truth.LineContext())
				// FinalizeStartTime must be set if state is updated.
				isFinalizing := expectedRow.FinalizationState == pb.WorkUnit_FINALIZING
				assert.Loosely(t, wuRow.FinalizeStartTime.Valid, should.Equal(isFinalizing), truth.LineContext())
				assert.Loosely(t, wuRow.FinalizerCandidateTime.Valid, should.Equal(isFinalizing), truth.LineContext())
				if isFinalizing {
					assert.Loosely(t, wuRow.FinalizeStartTime.Time, should.Match(wuRow.LastUpdated), truth.LineContext())
					assert.Loosely(t, wuRow.FinalizerCandidateTime.Time, should.Match(wuRow.LastUpdated), truth.LineContext())
				}

				// Match lastUpdated, etag, finalizeStartTime before comparing the full proto.
				expectedRowCopy := expectedRow.Clone()
				expectedRowCopy.LastUpdated = wuRow.LastUpdated
				expectedRowCopy.FinalizeStartTime = wuRow.FinalizeStartTime
				expectedRowCopy.FinalizerCandidateTime = wuRow.FinalizerCandidateTime
				// Validate WorkUnits table.
				assert.That(t, wuRow, should.Match(expectedRowCopy), truth.LineContext())

				// Validate legacy invocation table.
				inv, err := invocations.Read(span.Single(ctx), wuID.LegacyInvocationID(), invocations.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, inv, should.Match(expectedRowCopy.ToLegacyInvocationProto()))

				// The work unit update request should be recorded.
				exist, err := workunits.CheckWorkUnitUpdateRequestsExist(span.Single(ctx), []workunits.ID{wuID}, user, req.RequestId)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exist[wuID], should.BeTrue)
			}

			assertNoOp := func(respWU *pb.WorkUnit, expectedRow *workunits.WorkUnitRow, expected *pb.WorkUnit) {
				wuID := expectedRow.ID
				// Assert response.
				assert.That(t, respWU, should.Match(expected), truth.LineContext())
				// Assert spanner.
				wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil, truth.LineContext())
				assert.That(t, wuRow, should.Match(expectedRow), truth.LineContext())

				// The work unit update request should be recorded even for no-op.
				exist, err := workunits.CheckWorkUnitUpdateRequestsExist(span.Single(ctx), []workunits.ID{wuID}, user, "test-request-id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, exist[wuID], should.BeTrue)
			}

			t.Run("base case - no update", func(t *ftt.Test) {
				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assertNoOp(wu, expectedWURow, expectedWU)
			})

			t.Run("state, summary_markdown", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"state", "summary_markdown"}
				req.WorkUnit.State = pb.WorkUnit_FAILED
				req.WorkUnit.SummaryMarkdown = "The task failed because of..."

				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedWU.State = pb.WorkUnit_FAILED
				expectedWU.SummaryMarkdown = "The task failed because of..."
				expectedWU.FinalizationState = pb.WorkUnit_FINALIZING
				assertResponse(wu, expectedWU)

				// Validate work unit table.
				expectedWURow.State = pb.WorkUnit_FAILED
				expectedWURow.SummaryMarkdown = "The task failed because of..."
				expectedWURow.FinalizationState = pb.WorkUnit_FINALIZING
				assertSpannerRows(expectedWURow)

				// Enqueued the finalization task.
				expectedTasks := []protoreflect.ProtoMessage{
					&taskspb.SweepWorkUnitsForFinalization{RootInvocationId: string(rootInvID), SequenceNumber: 1},
				}
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
				// Finalizer task state updated on root invocation.
				taskState, err := rootinvocations.ReadFinalizerTaskState(span.Single(ctx), rootInvID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskState, should.Match(rootinvocations.FinalizerTaskState{Pending: true, Sequence: 1}))
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
					expectedWU.ExtendedProperties = extendedPropertiesNew
					assertResponse(wu, expectedWU)

					// Validate work unit table.
					expectedWURow.ExtendedProperties = extendedPropertiesNew
					assertSpannerRows(expectedWURow)
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
					expectedWU.ExtendedProperties = expectedExtendedProperties
					assertResponse(wu, expectedWU)

					// Validate work unit table.
					expectedWURow.ExtendedProperties = expectedExtendedProperties
					assertSpannerRows(expectedWURow)
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
					assert.Loosely(t, err, should.ErrLike(`bad request: work_unit: extended_properties: exceeds the maximum size of`))
					assert.Loosely(t, wu, should.BeNil)
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
						Name:              wuID.Name(),
						Deadline:          newDeadline,
						Properties:        newProperties,
						Instructions:      instruction,
						Tags:              []*pb.StringPair{{Key: "newkey", Value: "newvalue"}},
						FinalizationState: pb.WorkUnit_FINALIZING,
					},
					UpdateMask: updateMask,
					RequestId:  "test-request-id",
				}
				wu, err := recorder.UpdateWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				expectedWU.Deadline = newDeadline
				expectedWU.Properties = newProperties
				expectedWU.Instructions = instructionutil.InstructionsWithNames(instruction, wuID.Name())
				expectedWU.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
				assertResponse(wu, expectedWU)

				// Validate work unit table.
				expectedWURow.Deadline = newDeadline.AsTime()
				expectedWURow.Properties = newProperties
				expectedWURow.Instructions = instructionutil.InstructionsWithNames(instruction, wuID.Name())
				expectedWURow.Tags = []*pb.StringPair{{Key: "newkey", Value: "newvalue"}}
				assertSpannerRows(expectedWURow)
			})
		})
	})
}
