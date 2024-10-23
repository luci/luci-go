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
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateUpdateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	ftt.Run(`TestValidateUpdateInvocationRequest`, t, func(t *ftt.Test) {
		request := &pb.UpdateInvocationRequest{
			Invocation: &pb.Invocation{
				Name: "invocations/inv",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{}},
		}

		t.Run(`empty`, func(t *ftt.Test) {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{}, now)
			assert.Loosely(t, err, should.ErrLike(`invocation: name: unspecified`))
		})

		t.Run(`invalid id`, func(t *ftt.Test) {
			request.Invocation.Name = "1"
			err := validateUpdateInvocationRequest(request, now)
			assert.Loosely(t, err, should.ErrLike(`invocation: name: does not match`))
		})

		t.Run(`empty update mask`, func(t *ftt.Test) {
			err := validateUpdateInvocationRequest(request, now)
			assert.Loosely(t, err, should.ErrLike(`update_mask: paths is empty`))
		})

		t.Run(`unsupported update mask`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"name"}
			err := validateUpdateInvocationRequest(request, now)
			assert.Loosely(t, err, should.ErrLike(`update_mask: unsupported path "name"`))
		})

		t.Run(`submask in update mask`, func(t *ftt.Test) {
			t.Run(`unsupported`, func(t *ftt.Test) {
				request.UpdateMask.Paths = []string{"deadline.seconds"}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`update_mask: "deadline" should not have any submask`))
			})

			t.Run(`supported`, func(t *ftt.Test) {
				request.UpdateMask.Paths = []string{"extended_properties.some_key"}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run(`deadline`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"deadline"}

			t.Run(`invalid`, func(t *ftt.Test) {
				deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: deadline: must be at least 10 seconds in the future`))
			})

			t.Run(`valid`, func(t *ftt.Test) {
				deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run(`bigquery exports`, func(t *ftt.Test) {
			request.UpdateMask = &field_mask.FieldMask{Paths: []string{"bigquery_exports"}}

			t.Run(`invalid`, func(t *ftt.Test) {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					// No ResultType.
				}}
				request.UpdateMask.Paths = []string{"bigquery_exports"}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: bigquery_exports[0]: result_type: unspecified`))
			})

			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					ResultType: &pb.BigQueryExport_TestResults_{
						TestResults: &pb.BigQueryExport_TestResults{},
					},
				}}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`empty`, func(t *ftt.Test) {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run(`properties`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"properties"}

			t.Run(`invalid`, func(t *ftt.Test) {
				request.Invocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeInvocationProperties)),
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: properties: exceeds the maximum size of`))
				assert.Loosely(t, err, should.ErrLike(`bytes`))
			})
			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key_1": structpb.NewStringValue("value_1"),
						"key_2": structpb.NewStructValue(&structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewNumberValue(1),
							},
						}),
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run(`source spec`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"source_spec"}

			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.SourceSpec = &pb.SourceSpec{
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{
							Host:       "chromium.googlesource.com",
							Project:    "infra/infra",
							Ref:        "refs/heads/main",
							CommitHash: "1234567890abcdefabcd1234567890abcdefabcd",
							Position:   567,
						},
						Changelists: []*pb.GerritChange{
							{
								Host:     "chromium-review.googlesource.com",
								Project:  "infra/luci-go",
								Change:   12345,
								Patchset: 321,
							},
						},
						IsDirty: true,
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run(`invalid source spec`, func(t *ftt.Test) {
				request.Invocation.SourceSpec = &pb.SourceSpec{
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: source_spec: sources: gitiles_commit: host: unspecified`))
			})
		})
		t.Run(`is_source_spec_final`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"is_source_spec_final"}

			t.Run(`true`, func(t *ftt.Test) {
				request.Invocation.IsSourceSpecFinal = true
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`false`, func(t *ftt.Test) {
				// If the current field value is true and we are setting
				// false, a validation error is generated, but outside this
				// request validation routine.
				// For this purposes of this validation, this is not a
				// useful update to do, but it allowed to set a field to
				// its current value.
				request.Invocation.IsSourceSpecFinal = false
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
		})
		t.Run(`baseline_id`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"baseline_id"}

			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.BaselineId = "try:linux-rel"
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`empty`, func(t *ftt.Test) {
				request.Invocation.BaselineId = ""
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`invalid`, func(t *ftt.Test) {
				request.Invocation.BaselineId = "try/linux-rel"
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: baseline_id: does not match`))
			})
		})

		t.Run(`realm`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"realm"}

			t.Run(`empty`, func(t *ftt.Test) {
				request.Invocation.Realm = ""
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: realm: unspecified`))
			})
			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.Realm = "testproject:newrealm"
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`invalid`, func(t *ftt.Test) {
				request.Invocation.Realm = "blah"
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: realm: bad global realm name "blah"`))
			})
		})

		t.Run(`instructions`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"instructions"}

			t.Run(`empty`, func(t *ftt.Test) {
				request.Invocation.Instructions = nil
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`valid`, func(t *ftt.Test) {
				request.Invocation.Instructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id:              "step",
							Type:            pb.InstructionType_STEP_INSTRUCTION,
							DescriptiveName: "Step Instruction",
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
					}}
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`invalid`, func(t *ftt.Test) {
				request.Invocation.Instructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id:              "instruction1",
							Type:            pb.InstructionType_STEP_INSTRUCTION,
							DescriptiveName: "Step Instruction",
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
				err := validateUpdateInvocationRequest(request, now)
				assert.Loosely(t, err, should.ErrLike(`invocation: instructions: instructions[1]: id: "instruction1" is re-used at index 0`))
			})
		})

		t.Run(`extended properties`, func(t *ftt.Test) {
			t.Run(`full update mask`, func(t *ftt.Test) {
				request.UpdateMask.Paths = []string{"extended_properties"}

				t.Run(`empty`, func(t *ftt.Test) {
					request.Invocation.ExtendedProperties = nil
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`valid`, func(t *ftt.Test) {
					request.Invocation.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationExtendedPropertyValue)),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.ErrLike(`invocation: extended_properties: ["abc"]: exceeds the maximum size`))
					assert.Loosely(t, err, should.ErrLike(`bytes`))
				})
			})

			t.Run(`sub update mask`, func(t *ftt.Test) {

				t.Run(`backticks`, func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"extended_properties.`abc`"}
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
								"child_key": structpb.NewStringValue("child_value"),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`valid`, func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"extended_properties.abc"}
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
								"child_key": structpb.NewStringValue("child_value"),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`invalid`, func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"extended_properties.abc_"}
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc_": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"@type":     structpb.NewStringValue("foo.bar.com/x/some.package.MyMessage"),
								"child_key": structpb.NewStringValue("child_value"),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties: key "abc_": does not match`))
				})
				t.Run(`too deep`, func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"extended_properties.abc.fields"}
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewStringValue("child_value"),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					assert.Loosely(t, err, should.ErrLike(`update_mask: extended_properties["abc"] should not have any submask`))
				})
			})
		})
	})
}

func TestValidateUpdateInvocationPermissions(t *testing.T) {
	t.Parallel()
	ftt.Run(`TestValidateUpdateInvocationPermissions`, t, func(t *ftt.Test) {
		ctx := context.Background()
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				// permission required to update BigQuery exports.
				{Realm: "testproject:@root", Permission: permExportToBigQuery},
				// permission required to set baseline.
				{Realm: "testproject:@project", Permission: permPutBaseline},
				// permission required to change realm to newrealm.
				{Realm: "testproject:newrealm", Permission: permCreateInvocation},
				{Realm: "testproject:newrealm", Permission: permIncludeInvocation},
			},
		}
		ctx = auth.WithState(ctx, authState)

		request := &pb.UpdateInvocationRequest{
			Invocation: &pb.Invocation{
				Name: "invocations/inv",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{}},
		}

		existing := &pb.Invocation{
			Name:  "invocations/inv",
			Realm: "testproject:testrealm",
		}

		t.Run(`realm`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"realm"}
			request.Invocation.Realm = "testproject:newrealm"

			t.Run(`valid`, func(t *ftt.Test) {
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`no create access`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.PermissionDenied, `caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`))
			})
			t.Run(`no include access`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.PermissionDenied, `caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`))
			})
			t.Run(`change of project`, func(t *ftt.Test) {
				request.Invocation.Realm = "newproject:testrealm"
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, convey.Adapt(ShouldHaveAppStatus)(codes.InvalidArgument, `cannot change invocation realm to outside project "testproject"`))
			})
		})
		t.Run(`baseline_id`, func(t *ftt.Test) {
			request.UpdateMask.Paths = []string{"baseline_id"}
			request.Invocation.BaselineId = "try:linux-rel"

			t.Run(`empty`, func(t *ftt.Test) {
				request.Invocation.BaselineId = ""
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run(`no concurrent change to realm`, func(t *ftt.Test) {
				t.Run(`valid`, func(t *ftt.Test) {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`no access`, func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run(`concurrent change to realm`, func(t *ftt.Test) {
				request.UpdateMask.Paths = append(request.UpdateMask.Paths, "realm")
				request.Invocation.Realm = "testproject:newrealm"

				t.Run(`valid`, func(t *ftt.Test) {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run(`no access`, func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
		t.Run(`bigquery_exports`, func(t *ftt.Test) {
			request.UpdateMask.Paths = append(request.UpdateMask.Paths, "bigquery_exports")
			request.Invocation.BigqueryExports = []*pb.BigQueryExport{
				createTestBigQueryExportConfig(),
			}

			t.Run(`no permission`, func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permExportToBigQuery)

				t.Run(`with change`, func(t *ftt.Test) {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`updater does not have permission to set bigquery exports in realm "testproject:@root"`))
				})
				t.Run(`with no change`, func(t *ftt.Test) {
					// If we are not updating anything, we should not need permission.
					existing.BigqueryExports = []*pb.BigQueryExport{
						createTestBigQueryExportConfig(),
					}

					err := validateUpdateInvocationPermissions(ctx, existing, request)
					assert.Loosely(t, err, should.BeNil)
				})
			})
			t.Run(`valid`, func(t *ftt.Test) {
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}

func createTestBigQueryExportConfig() *pb.BigQueryExport {
	return &pb.BigQueryExport{
		Project: "my-project",
		Dataset: "my-dataset",
		Table:   "my-table",
		ResultType: &pb.BigQueryExport_TestResults_{
			TestResults: &pb.BigQueryExport_TestResults{
				Predicate: &pb.TestResultPredicate{
					TestIdRegexp: "regexp",
					Variant: &pb.VariantPredicate{
						Predicate: &pb.VariantPredicate_Contains{
							Contains: &pb.Variant{
								Def: map[string]string{"key": "value"},
							},
						},
					},
				},
			},
		},
	}
}

func TestUpdateInvocation(t *testing.T) {
	ftt.Run(`TestUpdateInvocation`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = tq.TestingContext(ctx, nil)
		start := clock.Now(ctx).UTC()

		recorder := newTestRecorderServer()

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:@project", Permission: permPutBaseline},
				{Realm: "testproject:@root", Permission: permExportToBigQuery},
				{Realm: "testproject:newrealm", Permission: permCreateInvocation},
				{Realm: "testproject:newrealm", Permission: permIncludeInvocation},
			},
		}
		ctx = auth.WithState(ctx, authState)

		t.Run(`invalid request`, func(t *ftt.Test) {
			req := &pb.UpdateInvocationRequest{}
			_, err := recorder.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`bad request: invocation: name: unspecified`))
		})
		t.Run(`no update token`, func(t *ftt.Test) {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCUnauthenticated)(`missing update-token metadata value in the request`))
		})
		t.Run(`invalid update token`, func(t *ftt.Test) {
			token, err := generateInvocationToken(ctx, "inv2")
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err = recorder.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`invalid update token`))
		})

		token, err := generateInvocationToken(ctx, "inv")
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("no invocation", func(t *ftt.Test) {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)(`invocations/inv not found`))
		})

		doInsert := func() {
			// Insert the invocation.
			testutil.MustApply(ctx, t, insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{
				"BaselineId": "existing-baseline",
			}))
		}

		t.Run("baseline_id", func(t *ftt.Test) {
			doInsert()
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					BaselineId: "try:linux-rel",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"baseline_id"}},
			}
			t.Run("without permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)

				inv, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				// the caller does not have permissions, so baseline should
				// be silently reset.
				assert.Loosely(t, inv.BaselineId, should.BeEmpty)
			})
			t.Run("with permission", func(t *ftt.Test) {
				inv, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.BaselineId, should.Equal("try:linux-rel"))
			})
		})

		t.Run("bigquery_exports", func(t *ftt.Test) {
			doInsert()
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					BigqueryExports: []*pb.BigQueryExport{
						createTestBigQueryExportConfig(),
					},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"bigquery_exports"}},
			}
			t.Run("without permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permExportToBigQuery)

				_, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`updater does not have permission to set bigquery exports in realm "testproject:@root"`))
			})
			t.Run("with permission", func(t *ftt.Test) {
				inv, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.BigqueryExports, should.Resemble([]*pb.BigQueryExport{
					createTestBigQueryExportConfig(),
				}))
			})
		})

		t.Run("realm", func(t *ftt.Test) {
			doInsert()
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:  "invocations/inv",
					Realm: "testproject:newrealm",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"realm"}},
			}

			t.Run("missing create invocation permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`))
			})
			t.Run("missing include invocation permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)(`caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`))
			})
			t.Run(`cannot change realm's project`, func(t *ftt.Test) {
				req.Invocation.Realm = "newproject:testrealm"

				_, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`cannot change invocation realm to outside project "testproject"`))
			})
			t.Run(`cannot change realm on an export root`, func(t *ftt.Test) {
				// Make the invocation an export root.
				testutil.MustApply(ctx, t, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId": invocations.ID("inv"),
					"IsExportRoot": true,
				}))

				_, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`realm: cannot change realm of an invocation that is an export root`))
			})
			t.Run(`valid`, func(t *ftt.Test) {
				inv, err := recorder.UpdateInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.Realm, should.Equal("testproject:newrealm"))
				assert.Loosely(t, inv.BaselineId, should.Equal("existing-baseline"))
			})
		})

		t.Run("is_source_spec_final", func(t *ftt.Test) {
			doInsert()
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"is_source_spec_final"}},
			}
			t.Run("from non-finalized sources", func(t *ftt.Test) {
				t.Run("to non-finalized sources", func(t *ftt.Test) {
					req.Invocation.IsSourceSpecFinal = false

					inv, err := recorder.UpdateInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.IsSourceSpecFinal, should.Equal(false))
				})
				t.Run("to finalized sources", func(t *ftt.Test) {
					req.Invocation.IsSourceSpecFinal = true

					inv, err := recorder.UpdateInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.IsSourceSpecFinal, should.Equal(true))
				})
			})
			t.Run("from finalized sources", func(t *ftt.Test) {
				testutil.MustApply(ctx, t, insert.Invocation("inv-sources-final", pb.Invocation_ACTIVE, map[string]any{
					"IsSourceSpecFinal": spanner.NullBool{Valid: true, Bool: true},
				}))
				req.Invocation.Name = "invocations/inv-sources-final"

				token, err := generateInvocationToken(ctx, "inv-sources-final")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

				t.Run("to finalized sources", func(t *ftt.Test) {
					req.Invocation.IsSourceSpecFinal = true

					inv, err := recorder.UpdateInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, inv.IsSourceSpecFinal, should.Equal(true))
				})
				t.Run("to non-finalized sources", func(t *ftt.Test) {
					req.Invocation.IsSourceSpecFinal = false

					_, err := recorder.UpdateInvocation(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`invocation: is_source_spec_final: cannot unfinalize already finalized sources`))
				})
			})
		})

		t.Run("extended_properties", func(t *ftt.Test) {
			doInsert()
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
			run := func(extendedPropertiesOrg map[string]*structpb.Struct, extendedPropertiesNew map[string]*structpb.Struct, updateMask *field_mask.FieldMask) (*pb.Invocation, error) {
				internalExtendedProperties := &invocationspb.ExtendedProperties{
					ExtendedProperties: extendedPropertiesOrg,
				}
				testutil.MustApply(ctx, t, insert.Invocation("update_extended_properties", pb.Invocation_ACTIVE, map[string]any{
					"ExtendedProperties": spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
				}))
				token, err := generateInvocationToken(ctx, "update_extended_properties")
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))
				req := &pb.UpdateInvocationRequest{
					Invocation: &pb.Invocation{
						Name:               "invocations/update_extended_properties",
						ExtendedProperties: extendedPropertiesNew,
					},
					UpdateMask: updateMask,
				}
				return recorder.UpdateInvocation(ctx, req)
			}

			t.Run("replace entire field", func(t *ftt.Test) {
				extendedPropertiesOrg := map[string]*structpb.Struct{
					"old_key": structValueOrg,
				}
				extendedPropertiesNew := map[string]*structpb.Struct{
					"new_key": structValueOrg,
				}
				updateMask := &field_mask.FieldMask{Paths: []string{"extended_properties"}}
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.ExtendedProperties, should.Resemble(extendedPropertiesNew))
			})
			t.Run("add keys to nil field", func(t *ftt.Test) {
				var extendedPropertiesOrg map[string]*structpb.Struct // a nil map
				extendedPropertiesNew := map[string]*structpb.Struct{
					"to_be_added_1": structValueNew,
					"to_be_added_2": structValueNew,
				}
				updateMask := &field_mask.FieldMask{Paths: []string{
					"extended_properties.to_be_added_1",
					"extended_properties.to_be_added_2",
				}}
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.ExtendedProperties, should.Resemble(extendedPropertiesNew))
			})
			t.Run("delete a key to nil field", func(t *ftt.Test) {
				var extendedPropertiesOrg map[string]*structpb.Struct // a nil map
				var extendedPropertiesNew map[string]*structpb.Struct // a nil map
				updateMask := &field_mask.FieldMask{Paths: []string{
					"extended_properties.to_be_deleted",
				}}
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.ExtendedProperties, should.BeEmpty)
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
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, inv.ExtendedProperties, should.Resemble(map[string]*structpb.Struct{
					"to_be_kept":     structValueOrg,
					"to_be_added":    structValueNew,
					"to_be_replaced": structValueNew,
				}))
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
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)(`invocation: extended_properties: exceeds the maximum size of`))
				assert.Loosely(t, inv, should.BeNil)
			})
		})

		t.Run("e2e", func(t *ftt.Test) {
			doInsert()
			validDeadline := pbutil.MustTimestampProto(start.Add(day))
			validBigqueryExports := []*pb.BigQueryExport{
				{
					Project: "project",
					Dataset: "dataset",
					Table:   "table1",
					ResultType: &pb.BigQueryExport_TestResults_{
						TestResults: &pb.BigQueryExport_TestResults{},
					},
				},
				{
					Project: "project",
					Dataset: "dataset",
					Table:   "table2",
					ResultType: &pb.BigQueryExport_TestResults_{
						TestResults: &pb.BigQueryExport_TestResults{},
					},
				},
			}

			instructions := &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:              "step",
						Type:            pb.InstructionType_STEP_INSTRUCTION,
						DescriptiveName: "Step instruction",
						Name:            "random",
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
						DescriptiveName: "Test instruction",
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
			}

			updateMask := &field_mask.FieldMask{
				Paths: []string{"deadline", "bigquery_exports", "properties", "is_source_spec_final", "source_spec", "baseline_id", "realm", "instructions"},
			}
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:            "invocations/inv",
					Deadline:        validDeadline,
					BigqueryExports: validBigqueryExports,
					Properties:      testutil.TestProperties(),
					SourceSpec: &pb.SourceSpec{
						Sources: testutil.TestSourcesWithChangelistNumbers(431, 123),
					},
					IsSourceSpecFinal: true,
					BaselineId:        "try:linux-rel",
					Realm:             "testproject:newrealm",
					Instructions:      instructions,
				},
				UpdateMask: updateMask,
			}
			inv, err := recorder.UpdateInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			expected := &pb.Invocation{
				Name:            "invocations/inv",
				Deadline:        validDeadline,
				BigqueryExports: validBigqueryExports,
				Properties:      testutil.TestProperties(),
				SourceSpec: &pb.SourceSpec{
					// The invocation should be stored and returned
					// normalized.
					Sources: testutil.TestSourcesWithChangelistNumbers(123, 431),
				},
				IsSourceSpecFinal: true,
				BaselineId:        "try:linux-rel",
				Realm:             "testproject:newrealm",
				Instructions:      instructionutil.InstructionsWithNames(instructions, "inv"),
			}
			assert.Loosely(t, inv.Name, should.Equal(expected.Name))
			assert.Loosely(t, inv.State, should.Equal(pb.Invocation_ACTIVE))
			assert.Loosely(t, inv.Deadline, should.Resemble(expected.Deadline))
			assert.Loosely(t, inv.Properties, should.Resemble(expected.Properties))
			assert.Loosely(t, inv.SourceSpec, should.Resemble(expected.SourceSpec))
			assert.Loosely(t, inv.IsSourceSpecFinal, should.Equal(expected.IsSourceSpecFinal))
			assert.Loosely(t, inv.BaselineId, should.Equal(expected.BaselineId))
			assert.Loosely(t, inv.Realm, should.Equal(expected.Realm))
			assert.Loosely(t, inv.Instructions, should.Resemble(expected.Instructions))

			// Read from the database.
			actual := &pb.Invocation{
				Name:       expected.Name,
				SourceSpec: &pb.SourceSpec{},
			}
			invID := invocations.ID("inv")
			var compressedProperties spanutil.Compressed
			var compressedSources spanutil.Compressed
			var isSourceSpecFinal spanner.NullBool
			var compressedInstructions spanutil.Compressed
			testutil.MustReadRow(ctx, t, "Invocations", invID.Key(), map[string]any{
				"Deadline":          &actual.Deadline,
				"BigQueryExports":   &actual.BigqueryExports,
				"Properties":        &compressedProperties,
				"Sources":           &compressedSources,
				"InheritSources":    &actual.SourceSpec.Inherit,
				"IsSourceSpecFinal": &isSourceSpecFinal,
				"BaselineId":        &actual.BaselineId,
				"Realm":             &actual.Realm,
				"Instructions":      &compressedInstructions,
			})
			actual.Properties = &structpb.Struct{}
			err = proto.Unmarshal(compressedProperties, actual.Properties)
			assert.Loosely(t, err, should.BeNil)
			actual.SourceSpec.Sources = &pb.Sources{}
			err = proto.Unmarshal(compressedSources, actual.SourceSpec.Sources)
			assert.Loosely(t, err, should.BeNil)
			actual.Instructions = &pb.Instructions{}
			err = proto.Unmarshal(compressedInstructions, actual.Instructions)
			assert.Loosely(t, err, should.BeNil)
			if isSourceSpecFinal.Valid && isSourceSpecFinal.Bool {
				actual.IsSourceSpecFinal = true
			}
			expected.Instructions = instructionutil.RemoveInstructionsName(instructions)
			assert.Loosely(t, actual, should.Resemble(expected))
		})
	})
}

func removePermission(perms []authtest.RealmPermission, permission realms.Permission) []authtest.RealmPermission {
	var result []authtest.RealmPermission
	for _, p := range perms {
		if p.Permission != permission {
			result = append(result, p)
		}
	}
	return result
}
