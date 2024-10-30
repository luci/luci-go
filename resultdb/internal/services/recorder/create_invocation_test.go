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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateInvocationDeadline(t *testing.T) {
	ftt.Run(`ValidateInvocationDeadline`, t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC

		t.Run(`deadline in the past`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateInvocationDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be at least 10 seconds in the future`))
		})

		t.Run(`deadline 5s in the future`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(5 * time.Second))
			err := validateInvocationDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be at least 10 seconds in the future`))
		})

		t.Run(`deadline in the future`, func(t *ftt.Test) {
			deadline := pbutil.MustTimestampProto(now.Add(1e3 * time.Hour))
			err := validateInvocationDeadline(deadline, now)
			assert.Loosely(t, err, should.ErrLike(`must be before 120h in the future`))
		})
	})
}

func TestVerifyCreateInvocationPermissions(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestVerifyCreateInvocationPermissions`, t, func(t *ftt.Test) {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "chromium:ci", Permission: permCreateInvocation},
			},
		})
		t.Run(`reserved prefix`, func(t *ftt.Test) {
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "build:8765432100",
				Invocation: &pb.Invocation{
					Realm: "chromium:ci",
				},
			})
			assert.Loosely(t, err, should.ErrLike(`only invocations created by trusted systems may have id not starting with "u-"`))
		})

		t.Run(`reserved prefix, allowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`producer_resource disallowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`only invocations created by trusted system may have a populated producer_resource field`))
		})

		t.Run(`producer_resource allowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`is_export_root allowed`, func(t *ftt.Test) {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
					{Realm: "chromium:ci", Permission: permSetExportRoot},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm:        "chromium:ci",
					IsExportRoot: true,
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`is_export_root disallowed`, func(t *ftt.Test) {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
				},
			})
			err := verifyCreateInvocationPermissions(ctx, &pb.CreateInvocationRequest{
				InvocationId: "u-abc",
				Invocation: &pb.Invocation{
					Realm:        "chromium:ci",
					IsExportRoot: true,
				},
			})
			assert.Loosely(t, err, should.ErrLike(`does not have permission to set export roots`))
		})
		t.Run(`bigquery_exports allowed`, func(t *ftt.Test) {
			ctx = auth.WithState(context.Background(), &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:ci", Permission: permCreateInvocation},
					{Realm: "chromium:@root", Permission: permExportToBigQuery},
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
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`bigquery_exports disallowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`does not have permission to set bigquery exports`))
		})
		t.Run(`baseline allowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run(`baseline disallowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`does not have permission to write to test baseline`))
		})
		t.Run(`creation disallowed`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.ErrLike(`does not have permission to create invocations`))
		})
		t.Run(`invalid realm`, func(t *ftt.Test) {
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
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invocation: realm: bad global realm name`))
		})
	})

}
func TestValidateCreateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	ftt.Run(`TestValidateCreateInvocationRequest`, t, func(t *ftt.Test) {
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

		t.Run(`valid`, func(t *ftt.Test) {
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`empty`, func(t *ftt.Test) {
			err := validateCreateInvocationRequest(&pb.CreateInvocationRequest{}, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation_id: unspecified`))
		})

		t.Run(`invalid id`, func(t *ftt.Test) {
			request.InvocationId = "1"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation_id: does not match`))
		})

		t.Run(`invalid request id`, func(t *ftt.Test) {
			request.RequestId = "😃"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike("request_id: does not match"))
		})

		t.Run(`invalid tags`, func(t *ftt.Test) {
			request.Invocation.Tags = pbutil.StringPairs("1", "a")
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation: tags: "1":"a": key: does not match`))
		})

		t.Run(`invalid deadline`, func(t *ftt.Test) {
			request.Invocation.Deadline = pbutil.MustTimestampProto(now.Add(-time.Hour))
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation: deadline: must be at least 10 seconds in the future`))
		})

		t.Run(`invalid realm`, func(t *ftt.Test) {
			request.Invocation.Realm = "B@d/f::rm@t"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation: realm: bad global realm name`))
		})

		t.Run(`invalid state`, func(t *ftt.Test) {
			request.Invocation.State = pb.Invocation_FINALIZED
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation: state: cannot be created in the state FINALIZED`))
		})

		t.Run(`invalid included invocation`, func(t *ftt.Test) {
			request.Invocation.IncludedInvocations = []string{"not an invocation name"}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`included_invocations[0]: invalid included invocation name`))
		})

		t.Run(`invalid bigqueryExports`, func(t *ftt.Test) {
			request.Invocation.BigqueryExports = []*pb.BigQueryExport{
				{
					Project: "project",
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`bigquery_export[0]: dataset: unspecified`))
		})

		t.Run(`invalid source spec`, func(t *ftt.Test) {
			request.Invocation.SourceSpec = &pb.SourceSpec{
				Sources: &pb.Sources{
					GitilesCommit: &pb.GitilesCommit{},
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`source_spec: sources: gitiles_commit: host: unspecified`))
		})

		t.Run(`invalid baseline`, func(t *ftt.Test) {
			request.Invocation.BaselineId = "try/linux-rel"
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`invocation: baseline_id: does not match`))
		})

		t.Run(`invalid properties`, func(t *ftt.Test) {
			request.Invocation.Properties = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationProperties)),
				},
			}
			err := validateCreateInvocationRequest(request, now, addedInvs)
			assert.Loosely(t, err, should.ErrLike(`properties: exceeds the maximum size of`))
			assert.Loosely(t, err, should.ErrLike(`bytes`))
		})

		t.Run(`invalid extended properties`, func(t *ftt.Test) {

			t.Run(`invalid key`, func(t *ftt.Test) {
				request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
					"mykey_": &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"child_key": structpb.NewStringValue("child_value"),
						},
					},
				}
				err := validateCreateInvocationRequest(request, now, addedInvs)
				assert.Loosely(t, err, should.ErrLike(`extended_properties: key "mykey_"`))
				assert.Loosely(t, err, should.ErrLike(`does not match`))
			})
			t.Run(`missing @type`, func(t *ftt.Test) {
				request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
					"mykey": &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"child_key": structpb.NewStringValue("child_value"),
						},
					},
				}
				err := validateCreateInvocationRequest(request, now, addedInvs)
				assert.Loosely(t, err, should.ErrLike(`extended_properties: ["mykey"]: must have a field "@type"`))
			})

			t.Run(`invalid @type`, func(t *ftt.Test) {
				request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
					"mykey": &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type":     structpb.NewStringValue("foo.bar.com/x/_some.package.MyMessage"),
							"child_key": structpb.NewStringValue("child_value"),
						},
					},
				}
				err := validateCreateInvocationRequest(request, now, addedInvs)
				assert.Loosely(t, err, should.ErrLike(`extended_properties: ["mykey"]: "@type" type name "_some.package.MyMessage": does not match`))
			})

			t.Run(`max size of value`, func(t *ftt.Test) {
				request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
					"mykey": &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
							"child_key": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationExtendedPropertyValue)),
						},
					},
				}
				err := validateCreateInvocationRequest(request, now, addedInvs)
				assert.Loosely(t, err, should.ErrLike(`extended_properties: ["mykey"]: exceeds the maximum size of`))
				assert.Loosely(t, err, should.ErrLike(`bytes`))
			})

			t.Run(`max size of extended properties`, func(t *ftt.Test) {
				tempValue := &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationExtendedPropertyValue-80)),
					},
				}
				request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
					"mykey_1": tempValue,
					"mykey_2": tempValue,
					"mykey_3": tempValue,
					"mykey_4": tempValue,
					"mykey_5": tempValue,
				}
				err := validateCreateInvocationRequest(request, now, addedInvs)
				assert.Loosely(t, err, should.ErrLike(`extended_properties: exceeds the maximum size of`))
				assert.Loosely(t, err, should.ErrLike(`bytes`))
			})
		})
	})
}

func TestCreateInvocation(t *testing.T) {
	ftt.Run(`TestCreateInvocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: permCreateInvocation},
				{Realm: "testproject:@root", Permission: permCreateWithReservedID},
				{Realm: "testproject:testrealm", Permission: permSetExportRoot},
				{Realm: "testproject:@root", Permission: permExportToBigQuery},
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
		assert.Loosely(t, err, should.BeNil)
		recorder := pb.NewRecorderPRPCClient(client)

		t.Run(`empty request`, func(t *ftt.Test) {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invocation: unspecified`))
		})
		t.Run(`invalid realm`, func(t *ftt.Test) {
			req := &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm: "testproject:",
				},
				RequestId: "request id",
			}
			_, err := recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invocation: realm`))
		})
		t.Run(`missing invocation id`, func(t *ftt.Test) {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
				Invocation: &pb.Invocation{
					Realm: "testproject:testrealm",
				},
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`invocation_id: unspecified`))
		})

		req := &pb.CreateInvocationRequest{
			InvocationId: "u-inv",
			Invocation: &pb.Invocation{
				Realm: "testproject:testrealm",
			},
		}

		t.Run(`already exists`, func(t *ftt.Test) {
			_, err := span.Apply(ctx, []*spanner.Mutation{
				insert.Invocation("u-inv", 1, nil),
			})
			assert.Loosely(t, err, should.BeNil)

			_, err = recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
		})

		t.Run(`unsorted tags`, func(t *ftt.Test) {
			req.Invocation.Tags = pbutil.StringPairs("b", "2", "a", "1")
			inv, err := recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv.Tags, should.Resemble(pbutil.StringPairs("a", "1", "b", "2")))
		})

		t.Run(`no invocation in request`, func(t *ftt.Test) {
			_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{InvocationId: "u-inv"})
			assert.Loosely(t, err, should.ErrLike("invocation: unspecified"))
		})

		t.Run(`invalid instructions`, func(t *ftt.Test) {
			req.Invocation.Instructions = &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:              "instruction1",
						Type:            pb.InstructionType_STEP_INSTRUCTION,
						DescriptiveName: "instruction 1",
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
								},
								Content: "content1",
							},
						},
					},
					{
						Id:                   "instruction1",
						TargetedInstructions: []*pb.TargetedInstruction{},
					},
				},
			}
			_, err := recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`instructions: instructions[1]: id: "instruction1" is re-used at index 0`))
		})

		t.Run(`idempotent`, func(t *ftt.Test) {
			req := &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm: "testproject:testrealm",
				},
				RequestId: "request id",
			}
			res, err := recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			res2, err := recorder.CreateInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res2, should.Resemble(res))
		})
		t.Run(`included invocation`, func(t *ftt.Test) {
			req = &pb.CreateInvocationRequest{
				InvocationId: "u-inv",
				Invocation: &pb.Invocation{
					Realm:               "testproject:testrealm",
					IncludedInvocations: []string{"invocations/u-inv-child"},
				},
			}
			t.Run(`non-existing invocation`, func(t *ftt.Test) {
				_, err := recorder.CreateInvocation(ctx, req)
				assert.Loosely(t, err, should.ErrLike("invocations/u-inv-child not found"))
			})
			t.Run(`non-permitted invocation`, func(t *ftt.Test) {
				incReq := &pb.CreateInvocationRequest{
					InvocationId: "u-inv-child",
					Invocation: &pb.Invocation{
						Realm: "testproject:createonly",
					},
				}
				_, err := recorder.CreateInvocation(ctx, incReq)
				assert.Loosely(t, err, should.BeNil)

				_, err = recorder.CreateInvocation(ctx, req)
				assert.Loosely(t, err, should.ErrLike("caller does not have permission resultdb.invocations.include"))
			})
			t.Run(`valid`, func(t *ftt.Test) {
				_, err := recorder.CreateInvocation(ctx, &pb.CreateInvocationRequest{
					InvocationId: "u-inv-child",
					Invocation: &pb.Invocation{
						Realm: "testproject:testrealm",
					},
				})
				assert.Loosely(t, err, should.BeNil)

				_, err = recorder.CreateInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				incIDs, err := invocations.ReadIncluded(span.Single(ctx), invocations.ID("u-inv"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, incIDs.Has(invocations.ID("u-inv-child")), should.BeTrue)
			})
		})

		t.Run(`end to end`, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

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
					Deadline:     deadline,
					Tags:         pbutil.StringPairs("a", "1", "b", "2"),
					IsExportRoot: true,
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
					IsSourceSpecFinal: true,
					BaselineId:        "testrealm:test-builder",
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
			}
			inv, err := recorder.CreateInvocation(ctx, req, grpc.Header(headers))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
				// For invocation finalizing.
				&taskspb.RunExportNotifications{InvocationId: "u-inv"},
				&taskspb.TryFinalizeInvocation{InvocationId: "u-inv"},
				// For invocation inclusion.
				&taskspb.RunExportNotifications{InvocationId: "u-inv", RootInvocationIds: []string{"u-inv"}, IncludedInvocationIds: []string{"u-inv-child"}},
			}))

			expected := proto.Clone(req.Invocation).(*pb.Invocation)
			expected.Instructions = instructionutil.InstructionsWithNames(expected.Instructions, "u-inv")
			proto.Merge(expected, &pb.Invocation{
				Name:      "invocations/u-inv",
				CreatedBy: "user:someone@example.com",

				// we use Spanner commit time, so skip the check
				CreateTime:        inv.CreateTime,
				FinalizeStartTime: inv.CreateTime,
			})
			assert.Loosely(t, inv, should.Resemble(expected))

			assert.Loosely(t, headers.Get(pb.UpdateTokenMetadataKey), should.HaveLength(1))

			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()

			inv, err = invocations.Read(ctx, "u-inv")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, inv, should.Resemble(expected))

			// Check fields not present in the proto.
			var invExpirationTime, expectedResultsExpirationTime time.Time
			err = invocations.ReadColumns(ctx, "u-inv", map[string]any{
				"InvocationExpirationTime":          &invExpirationTime,
				"ExpectedTestResultsExpirationTime": &expectedResultsExpirationTime,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expectedResultsExpirationTime, should.HappenWithin(time.Second, start.Add(expectedResultExpiration)))
			assert.Loosely(t, invExpirationTime, should.HappenWithin(time.Second, start.Add(invocationExpirationDuration)))
		})
	})
}
