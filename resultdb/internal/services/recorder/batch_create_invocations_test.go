// Copyright 2020 The LUCI Authors.
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
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateBatchCreateInvocationsRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC

	ftt.Run(`TestValidateBatchCreateInvocationsRequest`, t, func(t *ftt.Test) {
		t.Run(`invalid request id - Batch`, func(t *ftt.Test) {
			_, _, err := validateBatchCreateInvocationsRequest(
				now,
				[]*pb.CreateInvocationRequest{{
					InvocationId: "u-a",
					Invocation: &pb.Invocation{
						Realm: "testproject:testrealm",
					},
				}},
				"😃",
			)
			assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
		})
		t.Run(`non-matching request id - Batch`, func(t *ftt.Test) {
			_, _, err := validateBatchCreateInvocationsRequest(
				now,
				[]*pb.CreateInvocationRequest{{
					InvocationId: "u-a",
					Invocation: &pb.Invocation{
						Realm: "testproject:testrealm",
					},
					RequestId: "valid, but different"}},
				"valid",
			)
			assert.Loosely(t, err, should.ErrLike(`request_id: "valid" does not match`))
		})
		t.Run(`Too many requests`, func(t *ftt.Test) {
			_, _, err := validateBatchCreateInvocationsRequest(
				now,
				make([]*pb.CreateInvocationRequest, 1000),
				"valid",
			)
			assert.Loosely(t, err, should.ErrLike(`the number of requests in the batch`))
			assert.Loosely(t, err, should.ErrLike(`exceeds 500`))
		})
		t.Run(`valid`, func(t *ftt.Test) {
			ids, _, err := validateBatchCreateInvocationsRequest(
				now,
				[]*pb.CreateInvocationRequest{{
					InvocationId: "u-a",
					RequestId:    "valid",
					Invocation: &pb.Invocation{
						Realm: "testproject:testrealm",
					},
				}},
				"valid",
			)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ids.Has("u-a"), should.BeTrue)
			assert.Loosely(t, len(ids), should.Equal(1))
		})
	})
}

func TestBatchCreateInvocations(t *testing.T) {
	ftt.Run(`TestBatchCreateInvocations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = tq.TestingContext(ctx, nil)

		// Configure mock authentication to allow creation of custom invocation ids.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permCreateInvocation},
				{Realm: "testproject:testrealm", Permission: permSetExportRoot},
				{Realm: "testproject:@root", Permission: permExportToBigQuery},
				{Realm: "testproject:@root", Permission: permSetProducerResource},
				{Realm: "testproject:testrealm", Permission: permIncludeInvocation},
				{Realm: "testproject:createonly", Permission: permCreateInvocation},
				{Realm: "testproject:@project", Permission: permPutBaseline},
			},
		}
		ctx = auth.WithState(ctx, authState)

		start := clock.Now(ctx).UTC()

		// Setup a full HTTP server in order to retrieve response headers.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		t.Run(`idempotent`, func(t *ftt.Test) {
			req := &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{{
					InvocationId: "u-batchinv",
					Invocation:   &pb.Invocation{Realm: "testproject:testrealm"},
				}, {
					InvocationId: "u-batchinv2",
					Invocation:   &pb.Invocation{Realm: "testproject:testrealm"},
				}},
				RequestId: "request id",
			}
			res, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			res2, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			// Update tokens are regenerated the second time, but they are both valid.
			res2.UpdateTokens = res.UpdateTokens
			// Otherwise, the responses must be identical.
			assert.Loosely(t, res2, should.Resemble(res))
		})
		t.Run(`inclusion of non-existent invocation`, func(t *ftt.Test) {
			req := &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{{
					InvocationId: "u-batchinv",
					Invocation: &pb.Invocation{
						Realm:               "testproject:testrealm",
						IncludedInvocations: []string{"invocations/u-missing-inv"},
					},
				}, {
					InvocationId: "u-batchinv2",
					Invocation:   &pb.Invocation{Realm: "testproject:testrealm"},
				}},
			}
			_, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.ErrLike("invocations/u-missing-inv not found"))
		})

		t.Run(`inclusion of existing disallowed invocation`, func(t *ftt.Test) {
			req := &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{{
					InvocationId: "u-batchinv",
					Invocation:   &pb.Invocation{Realm: "testproject:createonly"},
				}},
			}
			_, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			req = &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{{
					InvocationId: "u-batchinv2",
					Invocation: &pb.Invocation{
						Realm:               "testproject:testrealm",
						IncludedInvocations: []string{"invocations/u-batchinv"},
					},
				}},
				RequestId: "request id",
			}
			_, err = recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.invocations.include"))
		})

		t.Run(`Same request ID, different identity`, func(t *ftt.Test) {
			req := &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{{
					InvocationId: "u-inv",
					Invocation:   &pb.Invocation{Realm: "testproject:testrealm"},
				}},
				RequestId: "request id",
			}
			_, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			authState.Identity = "user:someone-else@example.com"
			_, err = recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, status.Code(err), should.Equal(codes.AlreadyExists))
		})

		t.Run(`end to end`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(start.Add(time.Hour))
			bqExport := &pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			}
			req := &pb.BatchCreateInvocationsRequest{
				Requests: []*pb.CreateInvocationRequest{
					{
						InvocationId: "u-batch-inv",
						Invocation: &pb.Invocation{
							Deadline:     deadline,
							Tags:         pbutil.StringPairs("a", "1", "b", "2"),
							IsExportRoot: true,
							BigqueryExports: []*pb.BigQueryExport{
								bqExport,
							},
							ProducerResource:    "//builds.example.com/builds/1",
							Realm:               "testproject:testrealm",
							IncludedInvocations: []string{"invocations/u-batch-inv2"},
							Properties:          testutil.TestProperties(),
							SourceSpec: &pb.SourceSpec{
								Inherit: true,
							},
							IsSourceSpecFinal: true,
							BaselineId:        "testrealm:testbuilder",
							Instructions: &pb.Instructions{
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
									{
										Id:              "test",
										Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
										DescriptiveName: "Test Instruction",
										Name:            "random2",
										TargetedInstructions: []*pb.TargetedInstruction{
											{
												Targets: []pb.InstructionTarget{
													pb.InstructionTarget_LOCAL,
													pb.InstructionTarget_REMOTE,
												},
												Content: "test instruction",
												Dependencies: []*pb.InstructionDependency{
													{
														InvocationId:  "dep_inv_id",
														InstructionId: "dep_ins_id",
													},
												},
											},
										},
										InstructionFilter: &pb.InstructionFilter{
											FilterType: &pb.InstructionFilter_InvocationIds{
												InvocationIds: &pb.InstructionFilterByInvocationID{
													InvocationIds: []string{"swarming_task_1"},
												},
											},
										},
									},
								},
							},
							ExtendedProperties: testutil.TestInvocationExtendedProperties(),
						},
					},
					{
						InvocationId: "u-batch-inv2",
						Invocation: &pb.Invocation{
							Deadline: deadline,
							Tags:     pbutil.StringPairs("a", "1", "b", "2"),
							BigqueryExports: []*pb.BigQueryExport{
								bqExport,
							},
							ProducerResource: "//builds.example.com/builds/2",
							Realm:            "testproject:testrealm",
							Properties:       testutil.TestProperties(),
							SourceSpec: &pb.SourceSpec{
								Sources: testutil.TestSources(),
							},
						},
					},
				},
			}

			resp, err := recorder.BatchCreateInvocations(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			expected := proto.Clone(req.Requests[0].Invocation).(*pb.Invocation)
			expected.Instructions = instructionutil.InstructionsWithNames(expected.Instructions, "u-batch-inv")
			proto.Merge(expected, &pb.Invocation{
				Name:      "invocations/u-batch-inv",
				State:     pb.Invocation_ACTIVE,
				CreatedBy: "user:someone@example.com",

				// we use Spanner commit time, so skip the check
				CreateTime: resp.Invocations[0].CreateTime,
			})
			expected2 := proto.Clone(req.Requests[1].Invocation).(*pb.Invocation)
			expected2.Instructions = instructionutil.InstructionsWithNames(expected2.Instructions, "u-batch-inv2")
			proto.Merge(expected2, &pb.Invocation{
				Name:      "invocations/u-batch-inv2",
				State:     pb.Invocation_ACTIVE,
				CreatedBy: "user:someone@example.com",

				// we use Spanner commit time, so skip the check
				CreateTime: resp.Invocations[1].CreateTime,
			})
			assert.Loosely(t, resp.Invocations[0], should.Resemble(expected))
			assert.Loosely(t, resp.Invocations[1], should.Resemble(expected2))
			assert.Loosely(t, resp.UpdateTokens, should.HaveLength(2))

			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()

			inv, err := invocations.Read(ctx, "u-batch-inv")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Resemble(expected))

			inv2, err := invocations.Read(ctx, "u-batch-inv2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv2, should.Resemble(expected2))

			// Check fields not present in the proto.
			var invExpirationTime, expectedResultsExpirationTime time.Time
			err = invocations.ReadColumns(ctx, "u-batch-inv", map[string]any{
				"InvocationExpirationTime":          &invExpirationTime,
				"ExpectedTestResultsExpirationTime": &expectedResultsExpirationTime,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expectedResultsExpirationTime, should.HappenWithin(time.Second, start.Add(expectedResultExpiration)))
			assert.Loosely(t, invExpirationTime, should.HappenWithin(time.Second, start.Add(invocationExpirationDuration)))
			incIDs, err := invocations.ReadIncluded(ctx, invocations.ID("u-batch-inv"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, incIDs.Has(invocations.ID("u-batch-inv2")), should.BeTrue)
		})
	})
}
