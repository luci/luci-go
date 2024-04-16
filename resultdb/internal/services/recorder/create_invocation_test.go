// Copyright 2019 The LUCI Authors.
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
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateInvocationDeadline(t *testing.T) {
	Convey(`ValidateInvocationDeadline`, t, func() {
		now := testclock.TestRecentTimeUTC

		Convey(`deadline in the past`, func() {
			deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateInvocationDeadline(deadline, now)
			So(err, ShouldErrLike, `must be at least 10 seconds in the future`)
		})

		Convey(`deadline 5s in the future`, func() {
			deadline := pbutil.MustTimestampProto(now.Add(5 * time.Second))
			err := validateInvocationDeadline(deadline, now)
			So(err, ShouldErrLike, `must be at least 10 seconds in the future`)
		})

		Convey(`deadline in the future`, func() {
			deadline := pbutil.MustTimestampProto(now.Add(1e3 * time.Hour))
			err := validateInvocationDeadline(deadline, now)
			So(err, ShouldErrLike, `must be before 120h in the future`)
		})
	})
}

func TestVerifyCreateInvocationPermissions(t *testing.T) {
	t.Parallel()
	Convey(`TestVerifyCreateInvocationPermissions`, t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "chromium:ci", Permission: permCreateInvocation},
			},
		})
		Convey(`reserved prefix`, func() {
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "build:8765432100",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
				},
			})
			So(err, ShouldErrLike, `only invocations created by trusted systems may have id not starting with "u-"`)
		})

		Convey(`reserved prefix, allowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
					{Realm: "chromium:@root", Permission: permCreateWithReservedID},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "build:8765432100",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
				},
			})
			So(err, ShouldBeNil)
		})
		Convey(`producer_resource disallowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-0",
				Invocation: &pb.Invocation{
					Realm:            "chromium:ci",
					ProducerResource: "//builds.example.com/builds/1",
				},
			})
			So(err, ShouldErrLike, `only invocations created by trusted system may have a populated producer_resource field`)
		})

		Convey(`producer_resource allowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
					{Realm: "chromium:@root", Permission: permSetProducerResource},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-0",
				Invocation: &pb.Invocation{
					Realm:            "chromium:ci",
					ProducerResource: "//builds.example.com/builds/1",
				},
			})
			So(err, ShouldBeNil)
		})
		Convey(`bigquery_exports allowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
					{Realm: "chromium:ci", Permission: permExportToBigQuery},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
					BigqueryExports: []*pb.BigQueryExport{
						{
							Project: "project",
							Dataset: "dataset",
							Table:   "table",
							ResultType: &pb.BigQueryExport_TestResults_{
								TestResults: &pb.BigQueryExport_TestResults{},
							},
						},
					},
				},
			})
			So(err, ShouldBeNil)
		})
		Convey(`bigquery_exports disallowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
					BigqueryExports: []*pb.BigQueryExport{
						{
							Project: "project",
							Dataset: "dataset",
							Table:   "table",
							ResultType: &pb.BigQueryExport_TestResults_{
								TestResults: &pb.BigQueryExport_TestResults{},
							},
						},
					},
				},
			})
			So(err, ShouldErrLike, `does not have permission to set bigquery exports`)
		})
		Convey(`baseline allowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: permCreateInvocation},
					{Realm: "chromium:@project", Permission: permPutBaseline},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm:      "chromium:try",
					BaselineId: "try:linux-rel",
				},
			})
			So(err, ShouldBeNil)
		})
		Convey(`baseline disallowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: permCreateInvocation},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm:      "chromium:try",
					BaselineId: "try:linux-rel",
				},
			})
			So(err, ShouldErrLike, `does not have permission to write to test baseline`)
		})
		Convey(`creation disallowed`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity:            "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "build:8765432100",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
				},
			})
			So(err, ShouldErrLike, `does not have permission to create invocations`)
		})
		Convey(`invalid realm`, func() {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity:            "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "build:8765432100",
				Invocation: &pb.Invocation{
					Realm: "invalid:",
				},
			})
			So(err, ShouldHaveAppStatus, codes.InvalidArgument, `invocation: realm: bad global realm name`)
		})
	})

}
func TestValidateCreateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	Convey(`TestValidateCreateInvocationRequest`, t, func() {
		addedInvs := make(invocations.IDSet)
		deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
		request := &pb.CreateInvocationRequest{
			InvocationId: "u-abc",
			Invocation: &pb.Invocation{
				Deadline:            deadline,
				Tags:                pbutil.StringPairs("a", "b", "a", "c", "d", "e"),
				Realm:               "chromium:ci",
				IncludedInvocations: []string{"invocations/u-abc-2"},
				State:               pb.Invocation_FINALIZING,
			},
		}

		Convey(`valid`, func() {
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldBeNil)
		})

		Convey(`empty`, func() {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{}, now, addedInvs)
			So(err, ShouldErrLike, `invocation_id: unspecified`)
		})

		Convey(`invalid id`, func() {
			request.InvocationId = "1"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation_id: does not match`)
		})

		Convey(`invalid request id`, func() {
			request.RequestId = "😃"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, "request_id: does not match")
		})

		Convey(`invalid tags`, func() {
			request.Invocation.Tags = pbutil.StringPairs("1", "a")
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation: tags: "1":"a": key: does not match`)
		})

		Convey(`invalid deadline`, func() {
			request.Invocation.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation: deadline: must be at least 10 seconds in the future`)
		})

		Convey(`invalid realm`, func() {
			request.Invocation.Realm = "B@d/f::rm@t"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation: realm: bad global realm name`)
		})

		Convey(`invalid state`, func() {
			request.Invocation.State = pb.Invocation_FINALIZED
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation: state: cannot be created in the state FINALIZED`)
		})

		Convey(`invalid included invocation`, func() {
			request.Invocation.IncludedInvocations = []string{"not an invocation name"}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `included_invocations[0]: invalid included invocation name`)
		})

		Convey(`invalid bigqueryExports`, func() {
			request.Invocation.BigqueryExports = []*pb.BigQueryExport{
				{
					Project: "project",
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `bigquery_export[0]: dataset: unspecified`)
		})

		Convey(`invalid source spec`, func() {
			request.Invocation.SourceSpec = &pb.SourceSpec{
				Sources: &pb.Sources{
					GitilesCommit: &pb.GitilesCommit{},
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `source_spec: sources: gitiles_commit: host: unspecified`)
		})

		Convey(`invalid baseline`, func() {
			request.Invocation.BaselineId = "try/linux-rel"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `invocation: baseline_id: does not match`)
		})

		Convey(`invalid properties`, func() {
			request.Invocation.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			So(err, ShouldErrLike, `properties: exceeds the maximum size of`, `bytes`)
		})
	})
}

func TestCreateInvocation(t *testing.T) {
	Convey(`TestCreateInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permCreateInvocation},
				{Realm: "testproject:@root", Permission: permCreateWithReservedID},
				{Realm: "testproject:testrealm", Permission: permExportToBigQuery},
				{Realm: "testproject:@root", Permission: permSetProducerResource},
				{Realm: "testproject:testrealm", Permission: permIncludeInvocation},
				{Realm: "testproject:createonly", Permission: permCreateInvocation},
				{Realm: "testproject:@project", Permission: permPutBaseline},
			},
		})

		start := clock.Now(ctx).UTC()

		// Setup a full HTTP server in order to retrieve response headers.
		server := &prpctest.Server{}
		pb.RegisterRecorderServer(server, newTestRecorderServer())
		server.Start(ctx)
		defer server.Close()
		client, err := server.NewClient()
		So(err, ShouldBeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		Convey(`empty request`, func() {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{})
			So(err, ShouldBeRPCInvalidArgument, `invocation: unspecified`)
		})
		Convey(`invalid realm`, func() {
			req := &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm: "testproject:",
				},
				RequestId: "request id",
			}
			_, err := recorder.CreateInvocation(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `invocation: realm`)
		})
		Convey(`missing invocation id`, func() {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
				Invocation: &pb.Invocation{
					Realm: "testproject:testrealm",
				},
			})
			So(err, ShouldBeRPCInvalidArgument, `invocation_id: unspecified`)
		})

		req := &pb.CreateInvocationRequest{
			InvocationId: "u-inv",
			Invocation: &pb.Invocation{
				Realm: "testproject:testrealm",
			},
		}

		Convey(`already exists`, func() {
			_, err := span.Apply(ctx, []*spanner.Mutation{
				insert.Invocation("u-inv", 1, nil),
			})
			So(err, ShouldBeNil)

			_, err = recorder.CreateInvocation(ctx, req)
			So(err, ShouldBeRPCAlreadyExists)
		})

		Convey(`unsorted tags`, func() {
			req.Invocation.Tags = pbutil.StringPairs("b", "2", "a", "1")
			inv, err := recorder.CreateInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(inv.Tags, ShouldResemble, pbutil.StringPairs("a", "1", "b", "2"))
		})

		Convey(`no invocation in request`, func() {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{InvocationId: "u-inv"})
			So(err, ShouldErrLike, "invocation: unspecified")
		})

		Convey(`idempotent`, func() {
			req := &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm: "testproject:testrealm",
				},
				RequestId: "request id",
			}
			res, err := recorder.CreateInvocation(ctx, req)
			So(err, ShouldBeNil)

			res2, err := recorder.CreateInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(res2, ShouldResembleProto, res)
		})
		Convey(`included invocation`, func() {
			req = &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm:               "testproject:testrealm",
					IncludedInvocations: []string{"invocations/u-inv-child"},
				},
			}
			Convey(`non-existing invocation`, func() {
				_, err := recorder.CreateInvocation(ctx, req)
				So(err, ShouldErrLike, "invocations/u-inv-child not found")
			})
			Convey(`non-permitted invocation`, func() {
				incReq := &pb.CreateInvocationRequest{
					InvocationId: "u-inv-child",
					Invocation: &pb.Invocation{
						Realm: "testproject:createonly",
					},
				}
				_, err := recorder.CreateInvocation(ctx, incReq)
				So(err, ShouldBeNil)

				_, err = recorder.CreateInvocation(ctx, req)
				So(err, ShouldErrLike, "caller does not have permission resultdb.invocations.include")
			})
			Convey(`valid`, func() {
				_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
					InvocationId: "u-inv-child",
					Invocation: &pb.Invocation{
						Realm: "testproject:testrealm",
					},
				})
				So(err, ShouldBeNil)

				_, err = recorder.CreateInvocation(ctx, req)
				So(err, ShouldBeNil)

				incIDs, err := invocations.ReadIncluded(span.Single(ctx), invocations.ID("u-inv"))
				So(err, ShouldBeNil)
				So(incIDs.Has(invocations.ID("u-inv-child")), ShouldBeTrue)
			})
		})

		Convey(`end to end`, func() {
			deadline := pbutil.MustTimestampProto(start.Add(time.Hour))
			headers := &metadata.MD{}

			// Included invocation
			req := &pb.CreateInvocationRequest{
				InvocationId: "u-inv-child",
				Invocation: &pb.Invocation{
					Realm: "testproject:testrealm",
				},
			}
			_, err := recorder.CreateInvocation(ctx, req, grpc.Header(headers))
			So(err, ShouldBeNil)

			// Including invocation.
			bqExport := &pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			}

			req = &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Deadline: deadline,
					Tags:     pbutil.StringPairs("a", "1", "b", "2"),
					BigqueryExports: []*pb.BigQueryExport{
						bqExport,
					},
					ProducerResource:    "//builds.example.com/builds/1",
					Realm:               "testproject:testrealm",
					IncludedInvocations: []string{"invocations/u-inv-child"},
					State:               pb.Invocation_FINALIZING,
					Properties:          testutil.TestProperties(),
					SourceSpec: &pb.SourceSpec{
						Sources: testutil.TestSources(),
					},
					BaselineId: "testrealm:test-builder",
					TestInstruction: &pb.Instruction{
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
									pb.InstructionTarget_REMOTE,
								},
								Content: "test instruction",
								Dependency: []*pb.InstructionDependency{
									{
										BuildId:  "8000",
										StepName: "step",
									},
								},
							},
						},
					},
					StepInstructions: &pb.Instructions{
						Instructions: []*pb.Instruction{
							{
								Id: "step",
								TargetedInstructions: []*pb.TargetedInstruction{
									{
										Targets: []pb.InstructionTarget{
											pb.InstructionTarget_LOCAL,
											pb.InstructionTarget_REMOTE,
										},
										Content: "step instruction",
										Dependency: []*pb.InstructionDependency{
											{
												BuildId:  "8001",
												StepName: "dep_step",
											},
										},
									},
								},
							},
						},
					},
				},
			}
			inv, err := recorder.CreateInvocation(ctx, req, grpc.Header(headers))
			So(err, ShouldBeNil)
			So(sched.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
				&taskspb.TryFinalizeInvocation{InvocationId: "u-inv"},
			})

			expected := proto.Clone(req.Invocation).(*pb.Invocation)
			proto.Merge(expected, &pb.Invocation{
				Name:      "invocations/u-inv",
				CreatedBy: "user:someone@example.com",

				// we use Spanner commit time, so skip the check
				CreateTime:        inv.CreateTime,
				FinalizeStartTime: inv.CreateTime,
			})
			So(inv, ShouldResembleProto, expected)

			So(headers.Get(pb.UpdateTokenMetadataKey), ShouldHaveLength, 1)

			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()

			inv, err = invocations.Read(ctx, "u-inv")
			So(err, ShouldBeNil)
			So(inv, ShouldResembleProto, expected)

			// Check fields not present in the proto.
			var invExpirationTime, expectedResultsExpirationTime time.Time
			err = invocations.ReadColumns(ctx, "u-inv", map[string]any{
				"InvocationExpirationTime":          &invExpirationTime,
				"ExpectedTestResultsExpirationTime": &expectedResultsExpirationTime,
			})
			So(err, ShouldBeNil)
			So(expectedResultsExpirationTime, ShouldHappenWithin, time.Second, start.Add(expectedResultExpiration))
			So(invExpirationTime, ShouldHappenWithin, time.Second, start.Add(invocationExpirationDuration))
		})
	})
}
