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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateUpdateInvocationRequest(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	Convey(`TestValidateUpdateInvocationRequest`, t, func() {
		request := &pb.UpdateInvocationRequest{
			Invocation: &pb.Invocation{
				Name: "invocations/inv",
			},
			UpdateMask: &field_mask.FieldMask{Paths: []string{}},
		}

		Convey(`empty`, func() {
			err := validateUpdateInvocationRequest(&pb.UpdateInvocationRequest{}, now)
			So(err, ShouldErrLike, `invocation: name: unspecified`)
		})

		Convey(`invalid id`, func() {
			request.Invocation.Name = "1"
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `invocation: name: does not match`)
		})

		Convey(`empty update mask`, func() {
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `update_mask: paths is empty`)
		})

		Convey(`unsupported update mask`, func() {
			request.UpdateMask.Paths = []string{"name"}
			err := validateUpdateInvocationRequest(request, now)
			So(err, ShouldErrLike, `update_mask: unsupported path "name"`)
		})

		Convey(`submask in update mask`, func() {
			Convey(`unsupported`, func() {
				request.UpdateMask.Paths = []string{"deadline.seconds"}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `update_mask: "deadline" should not have any submask`)
			})

			Convey(`supported`, func() {
				request.UpdateMask.Paths = []string{"extended_properties.some_key"}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})

		Convey(`deadline`, func() {
			request.UpdateMask.Paths = []string{"deadline"}

			Convey(`invalid`, func() {
				deadline := pbutil.MustTimestampProto(now.Add(-time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: deadline: must be at least 10 seconds in the future`)
			})

			Convey(`valid`, func() {
				deadline := pbutil.MustTimestampProto(now.Add(time.Hour))
				request.Invocation.Deadline = deadline
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`bigquery exports`, func() {
			request.UpdateMask = &field_mask.FieldMask{Paths: []string{"bigquery_exports"}}

			Convey(`invalid`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					// No ResultType.
				}}
				request.UpdateMask.Paths = []string{"bigquery_exports"}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: bigquery_exports[0]: result_type: unspecified`)
			})

			Convey(`valid`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{{
					Project: "project",
					Dataset: "dataset",
					Table:   "table",
					ResultType: &pb.BigQueryExport_TestResults_{
						TestResults: &pb.BigQueryExport_TestResults{},
					},
				}}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})

			Convey(`empty`, func() {
				request.Invocation.BigqueryExports = []*pb.BigQueryExport{}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`properties`, func() {
			request.UpdateMask.Paths = []string{"properties"}

			Convey(`invalid`, func() {
				request.Invocation.Properties = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key1": structpb.NewStringValue(strings.Repeat("1", pbutil.MaxSizeInvocationProperties)),
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: properties: exceeds the maximum size of`, `bytes`)
			})
			Convey(`valid`, func() {
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
				So(err, ShouldBeNil)
			})
		})
		Convey(`source spec`, func() {
			request.UpdateMask.Paths = []string{"source_spec"}

			Convey(`valid`, func() {
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
				So(err, ShouldBeNil)
			})

			Convey(`invalid source spec`, func() {
				request.Invocation.SourceSpec = &pb.SourceSpec{
					Sources: &pb.Sources{
						GitilesCommit: &pb.GitilesCommit{},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: source_spec: sources: gitiles_commit: host: unspecified`)
			})
		})
		Convey(`is_source_spec_final`, func() {
			request.UpdateMask.Paths = []string{"is_source_spec_final"}

			Convey(`true`, func() {
				request.Invocation.IsSourceSpecFinal = true
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`false`, func() {
				// If the current field value is true and we are setting
				// false, a validation error is generated, but outside this
				// request validation routine.
				// For this purposes of this validation, this is not a
				// useful update to do, but it allowed to set a field to
				// its current value.
				request.Invocation.IsSourceSpecFinal = false
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
		})
		Convey(`baseline_id`, func() {
			request.UpdateMask.Paths = []string{"baseline_id"}

			Convey(`valid`, func() {
				request.Invocation.BaselineId = "try:linux-rel"
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`empty`, func() {
				request.Invocation.BaselineId = ""
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
				request.Invocation.BaselineId = "try/linux-rel"
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: baseline_id: does not match`)
			})
		})

		Convey(`realm`, func() {
			request.UpdateMask.Paths = []string{"realm"}

			Convey(`empty`, func() {
				request.Invocation.Realm = ""
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: realm: unspecified`)
			})
			Convey(`valid`, func() {
				request.Invocation.Realm = "testproject:newrealm"
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
				request.Invocation.Realm = "blah"
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: realm: bad global realm name "blah"`)
			})
		})

		Convey(`instructions`, func() {
			request.UpdateMask.Paths = []string{"instructions"}

			Convey(`empty`, func() {
				request.Invocation.Instructions = nil
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`valid`, func() {
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
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
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
				So(err, ShouldErrLike, `invocation: instructions: instructions[1]: id: "instruction1" is re-used at index 0`)
			})
		})

		Convey(`extended properties`, func() {
			Convey(`full update mask`, func() {
				request.UpdateMask.Paths = []string{"extended_properties"}

				Convey(`empty`, func() {
					request.Invocation.ExtendedProperties = nil
					err := validateUpdateInvocationRequest(request, now)
					So(err, ShouldBeNil)
				})
				Convey(`valid`, func() {
					request.Invocation.ExtendedProperties = testutil.TestInvocationExtendedProperties()
					err := validateUpdateInvocationRequest(request, now)
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewStringValue(strings.Repeat("a", pbutil.MaxSizeInvocationExtendedPropertyValue)),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					So(err, ShouldErrLike, `invocation: extended_properties: ["abc"]: exceeds the maximum size`, `bytes`)
				})
			})

			Convey(`sub update mask`, func() {

				Convey(`backticks`, func() {
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
					So(err, ShouldBeNil)
				})
				Convey(`valid`, func() {
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
					So(err, ShouldBeNil)
				})
				Convey(`invalid`, func() {
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
					So(err, ShouldErrLike, `update_mask: extended_properties: key "abc_": does not match`)
				})
				Convey(`too deep`, func() {
					request.UpdateMask.Paths = []string{"extended_properties.abc.fields"}
					request.Invocation.ExtendedProperties = map[string]*structpb.Struct{
						"abc": &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"child_key": structpb.NewStringValue("child_value"),
							},
						},
					}
					err := validateUpdateInvocationRequest(request, now)
					So(err, ShouldErrLike, `update_mask: extended_properties["abc"] should not have any submask`)
				})
			})
		})
	})
}

func TestValidateUpdateInvocationPermissions(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateUpdateInvocationPermissions`, t, func() {
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

		Convey(`realm`, func() {
			request.UpdateMask.Paths = []string{"realm"}
			request.Invocation.Realm = "testproject:newrealm"

			Convey(`valid`, func() {
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldBeNil)
			})
			Convey(`no create access`, func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`no include access`, func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`change of project`, func() {
				request.Invocation.Realm = "newproject:testrealm"
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, `cannot change invocation realm to outside project "testproject"`)
			})
		})
		Convey(`baseline_id`, func() {
			request.UpdateMask.Paths = []string{"baseline_id"}
			request.Invocation.BaselineId = "try:linux-rel"

			Convey(`empty`, func() {
				request.Invocation.BaselineId = ""
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldBeNil)
			})
			Convey(`no concurrent change to realm`, func() {
				Convey(`valid`, func() {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					So(err, ShouldBeNil)
				})
				Convey(`no access`, func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					So(err, ShouldBeNil)
				})
			})
			Convey(`concurrent change to realm`, func() {
				request.UpdateMask.Paths = append(request.UpdateMask.Paths, "realm")
				request.Invocation.Realm = "testproject:newrealm"

				Convey(`valid`, func() {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					So(err, ShouldBeNil)
				})
				Convey(`no access`, func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					So(err, ShouldBeNil)
				})
			})
		})
		Convey(`bigquery_exports`, func() {
			request.UpdateMask.Paths = append(request.UpdateMask.Paths, "bigquery_exports")
			request.Invocation.BigqueryExports = []*pb.BigQueryExport{
				createTestBigQueryExportConfig(),
			}

			Convey(`no permission`, func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permExportToBigQuery)

				Convey(`with change`, func() {
					err := validateUpdateInvocationPermissions(ctx, existing, request)
					So(err, ShouldBeRPCPermissionDenied, `updater does not have permission to set bigquery exports in realm "testproject:@root"`)
				})
				Convey(`with no change`, func() {
					// If we are not updating anything, we should not need permission.
					existing.BigqueryExports = []*pb.BigQueryExport{
						createTestBigQueryExportConfig(),
					}

					err := validateUpdateInvocationPermissions(ctx, existing, request)
					So(err, ShouldBeNil)
				})
			})
			Convey(`valid`, func() {
				err := validateUpdateInvocationPermissions(ctx, existing, request)
				So(err, ShouldBeNil)
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
	Convey(`TestUpdateInvocation`, t, func() {
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

		Convey(`invalid request`, func() {
			req := &pb.UpdateInvocationRequest{}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeRPCInvalidArgument, `bad request: invocation: name: unspecified`)
		})
		Convey(`no update token`, func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeRPCUnauthenticated, `missing update-token metadata value in the request`)
		})
		Convey(`invalid update token`, func() {
			token, err := generateInvocationToken(ctx, "inv2")
			So(err, ShouldBeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err = recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeRPCPermissionDenied, `invalid update token`)
		})

		token, err := generateInvocationToken(ctx, "inv")
		So(err, ShouldBeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		Convey("no invocation", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					Properties: testutil.TestProperties(),
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"properties"}},
			}
			_, err := recorder.UpdateInvocation(ctx, req)
			So(err, ShouldBeRPCNotFound, `invocations/inv not found`)
		})

		// Insert the invocation.
		testutil.MustApply(ctx, insert.Invocation("inv", pb.Invocation_ACTIVE, map[string]any{
			"BaselineId": "existing-baseline",
		}))

		Convey("baseline_id", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:       "invocations/inv",
					BaselineId: "try:linux-rel",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"baseline_id"}},
			}
			Convey("without permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)

				inv, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeNil)
				// the caller does not have permissions, so baseline should
				// be silently reset.
				So(inv.BaselineId, ShouldEqual, "")
			})
			Convey("with permission", func() {
				inv, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeNil)
				So(inv.BaselineId, ShouldEqual, "try:linux-rel")
			})
		})
		Convey("bigquery_exports", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
					BigqueryExports: []*pb.BigQueryExport{
						createTestBigQueryExportConfig(),
					},
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"bigquery_exports"}},
			}
			Convey("without permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permExportToBigQuery)

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `updater does not have permission to set bigquery exports in realm "testproject:@root"`)
			})
			Convey("with permission", func() {
				inv, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeNil)
				So(inv.BigqueryExports, ShouldResembleProto, []*pb.BigQueryExport{
					createTestBigQueryExportConfig(),
				})
			})
		})
		Convey("realm", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:  "invocations/inv",
					Realm: "testproject:newrealm",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"realm"}},
			}

			Convey("missing create invocation permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey("missing include invocation permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`cannot change realm's project`, func() {
				req.Invocation.Realm = "newproject:testrealm"

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, `cannot change invocation realm to outside project "testproject"`)
			})
			Convey(`cannot change realm on an export root`, func() {
				// Make the invocation an export root.
				testutil.MustApply(ctx, spanutil.UpdateMap("Invocations", map[string]any{
					"InvocationId": invocations.ID("inv"),
					"IsExportRoot": true,
				}))

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, `realm: cannot change realm of an invocation that is an export root`)
			})
			Convey(`valid`, func() {
				inv, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeNil)
				So(inv.Realm, ShouldEqual, "testproject:newrealm")
				So(inv.BaselineId, ShouldEqual, "existing-baseline")
			})
		})
		Convey("is_source_spec_final", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name: "invocations/inv",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"is_source_spec_final"}},
			}
			Convey("from non-finalized sources", func() {
				Convey("to non-finalized sources", func() {
					req.Invocation.IsSourceSpecFinal = false

					inv, err := recorder.UpdateInvocation(ctx, req)
					So(err, ShouldBeNil)
					So(inv.IsSourceSpecFinal, ShouldEqual, false)
				})
				Convey("to finalized sources", func() {
					req.Invocation.IsSourceSpecFinal = true

					inv, err := recorder.UpdateInvocation(ctx, req)
					So(err, ShouldBeNil)
					So(inv.IsSourceSpecFinal, ShouldEqual, true)
				})
			})
			Convey("from finalized sources", func() {
				testutil.MustApply(ctx, insert.Invocation("inv-sources-final", pb.Invocation_ACTIVE, map[string]any{
					"IsSourceSpecFinal": spanner.NullBool{Valid: true, Bool: true},
				}))
				req.Invocation.Name = "invocations/inv-sources-final"

				token, err := generateInvocationToken(ctx, "inv-sources-final")
				So(err, ShouldBeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

				Convey("to finalized sources", func() {
					req.Invocation.IsSourceSpecFinal = true

					inv, err := recorder.UpdateInvocation(ctx, req)
					So(err, ShouldBeNil)
					So(inv.IsSourceSpecFinal, ShouldEqual, true)
				})
				Convey("to non-finalized sources", func() {
					req.Invocation.IsSourceSpecFinal = false

					_, err := recorder.UpdateInvocation(ctx, req)
					So(err, ShouldBeRPCInvalidArgument, `invocation: is_source_spec_final: cannot unfinalize already finalized sources`)
				})
			})
		})
		Convey("extended_properties", func() {
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
				testutil.MustApply(ctx, insert.Invocation("update_extended_properties", pb.Invocation_ACTIVE, map[string]any{
					"ExtendedProperties": spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
				}))
				token, err := generateInvocationToken(ctx, "update_extended_properties")
				So(err, ShouldBeNil)
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

			Convey("replace entire field", func() {
				extendedPropertiesOrg := map[string]*structpb.Struct{
					"old_key": structValueOrg,
				}
				extendedPropertiesNew := map[string]*structpb.Struct{
					"new_key": structValueOrg,
				}
				updateMask := &field_mask.FieldMask{Paths: []string{"extended_properties"}}
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				So(err, ShouldBeNil)
				So(inv.ExtendedProperties, ShouldResembleProto, extendedPropertiesNew)
			})
			Convey("add keys to nil field", func() {
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
				So(err, ShouldBeNil)
				So(inv.ExtendedProperties, ShouldResembleProto, extendedPropertiesNew)
			})
			Convey("delete a key to nil field", func() {
				var extendedPropertiesOrg map[string]*structpb.Struct // a nil map
				var extendedPropertiesNew map[string]*structpb.Struct // a nil map
				updateMask := &field_mask.FieldMask{Paths: []string{
					"extended_properties.to_be_deleted",
				}}
				inv, err := run(extendedPropertiesOrg, extendedPropertiesNew, updateMask)
				So(err, ShouldBeNil)
				So(inv.ExtendedProperties, ShouldBeEmpty)
			})
			Convey("add, replace, and delete keys to existing field", func() {
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
				So(err, ShouldBeNil)
				So(inv.ExtendedProperties, ShouldResembleProto, map[string]*structpb.Struct{
					"to_be_kept":     structValueOrg,
					"to_be_added":    structValueNew,
					"to_be_replaced": structValueNew,
				})
			})
			Convey("valid request but overall size exceed limit", func() {
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
				So(err, ShouldBeRPCInvalidArgument, `invocation: extended_properties: exceeds the maximum size of`)
				So(inv, ShouldBeNil)
			})
		})

		Convey("e2e", func() {
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
			So(err, ShouldBeNil)

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
			So(inv.Name, ShouldEqual, expected.Name)
			So(inv.State, ShouldEqual, pb.Invocation_ACTIVE)
			So(inv.Deadline, ShouldResembleProto, expected.Deadline)
			So(inv.Properties, ShouldResembleProto, expected.Properties)
			So(inv.SourceSpec, ShouldResembleProto, expected.SourceSpec)
			So(inv.IsSourceSpecFinal, ShouldEqual, expected.IsSourceSpecFinal)
			So(inv.BaselineId, ShouldEqual, expected.BaselineId)
			So(inv.Realm, ShouldEqual, expected.Realm)
			So(inv.Instructions, ShouldResembleProto, expected.Instructions)

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
			testutil.MustReadRow(ctx, "Invocations", invID.Key(), map[string]any{
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
			So(err, ShouldBeNil)
			actual.SourceSpec.Sources = &pb.Sources{}
			err = proto.Unmarshal(compressedSources, actual.SourceSpec.Sources)
			So(err, ShouldBeNil)
			actual.Instructions = &pb.Instructions{}
			err = proto.Unmarshal(compressedInstructions, actual.Instructions)
			So(err, ShouldBeNil)
			if isSourceSpecFinal.Valid && isSourceSpecFinal.Bool {
				actual.IsSourceSpecFinal = true
			}
			expected.Instructions = instructionutil.RemoveInstructionsName(instructions)
			So(actual, ShouldResembleProto, expected)
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
