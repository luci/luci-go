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

	"go.chromium.org/luci/resultdb/internal/invocations"
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

		Convey(`test instruction`, func() {
			request.UpdateMask.Paths = []string{"test_instruction"}

			Convey(`empty`, func() {
				request.Invocation.TestInstruction = nil
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`valid`, func() {
				request.Invocation.TestInstruction = &pb.Instruction{
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content1",
						},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
				request.Invocation.TestInstruction = &pb.Instruction{
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content1",
						},
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "content1",
						},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldErrLike, `invocation: test_instruction: test instruction: target: duplicated`)
			})
		})

		Convey(`step instructions`, func() {
			request.UpdateMask.Paths = []string{"step_instructions"}

			Convey(`empty`, func() {
				request.Invocation.StepInstructions = nil
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`valid`, func() {
				request.Invocation.StepInstructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id: "instruction1",
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
							Id: "instruction2",
							TargetedInstructions: []*pb.TargetedInstruction{
								{
									Targets: []pb.InstructionTarget{
										pb.InstructionTarget_LOCAL,
									},
									Content: "content2",
								},
							},
						},
					},
				}
				err := validateUpdateInvocationRequest(request, now)
				So(err, ShouldBeNil)
			})
			Convey(`invalid`, func() {
				request.Invocation.StepInstructions = &pb.Instructions{
					Instructions: []*pb.Instruction{
						{
							Id: "instruction1",
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
				So(err, ShouldErrLike, `invocation: step_instructions: step instructions: ID "instruction1" is re-used at index 0 and 1`)
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
				// permission required to set baseline
				{Realm: "testproject:@project", Permission: permPutBaseline},
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

		existingRealm := "testproject:testrealm"

		Convey(`realm`, func() {
			request.UpdateMask.Paths = []string{"realm"}
			request.Invocation.Realm = "testproject:newrealm"

			Convey(`valid`, func() {
				err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
				So(err, ShouldBeNil)
			})
			Convey(`no create access`, func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)
				err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`no include access`, func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)
				err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
				So(err, ShouldHaveAppStatus, codes.PermissionDenied, `caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`change of project`, func() {
				request.Invocation.Realm = "newproject:testrealm"
				err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
				So(err, ShouldHaveAppStatus, codes.InvalidArgument, `cannot change invocation realm to outside project "testproject"`)
			})
		})
		Convey(`baseline_id`, func() {
			request.UpdateMask.Paths = []string{"baseline_id"}
			request.Invocation.BaselineId = "try:linux-rel"

			Convey(`empty`, func() {
				request.Invocation.BaselineId = ""
				err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
				So(err, ShouldBeNil)
			})
			Convey(`no concurrent change to realm`, func() {
				Convey(`valid`, func() {
					err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
					So(err, ShouldBeNil)
				})
				Convey(`no access`, func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					So(err, ShouldBeNil)
				})
			})
			Convey(`concurrent change to realm`, func() {
				request.UpdateMask.Paths = append(request.UpdateMask.Paths, "realm")
				request.Invocation.Realm = "testproject:newrealm"

				Convey(`valid`, func() {
					err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
					So(err, ShouldBeNil)
				})
				Convey(`no access`, func() {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)
					err := validateUpdateInvocationPermissions(ctx, existingRealm, request)
					// TODO: Once we stop silently swallowing errors, expect a non-nil result.
					So(err, ShouldBeNil)
				})
			})
		})
	})
}

func TestUpdateInvocation(t *testing.T) {
	Convey(`TestUpdateInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		start := clock.Now(ctx).UTC()

		recorder := newTestRecorderServer()

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:@project", Permission: permPutBaseline},
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
		Convey("realm", func() {
			req := &pb.UpdateInvocationRequest{
				Invocation: &pb.Invocation{
					Name:  "invocations/inv",
					Realm: "testproject:newrealm",
				},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"realm"}},
			}
			Convey("without create invocation permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permCreateInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission to create invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey("without include invocation permission", func() {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permIncludeInvocation)

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCPermissionDenied, `caller does not have permission to include invocations in realm "testproject:newrealm" (required to update invocation realm)`)
			})
			Convey(`to new project`, func() {
				req.Invocation.Realm = "newproject:testrealm"

				_, err := recorder.UpdateInvocation(ctx, req)
				So(err, ShouldBeRPCInvalidArgument, `cannot change invocation realm to outside project "testproject"`)
			})
			Convey(`valid`, func() {
				// Regardless of permission to put baseline.
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, permPutBaseline)

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

			stepInstructions := &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id: "instruction1",
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
						Id: "instruction2",
						TargetedInstructions: []*pb.TargetedInstruction{
							{
								Targets: []pb.InstructionTarget{
									pb.InstructionTarget_LOCAL,
								},
								Content: "content2",
							},
						},
					},
				},
			}

			testInstruction := &pb.Instruction{
				TargetedInstructions: []*pb.TargetedInstruction{
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_LOCAL,
						},
						Content: "content1",
					},
					{
						Targets: []pb.InstructionTarget{
							pb.InstructionTarget_REMOTE,
						},
						Content: "content2",
						Dependency: []*pb.InstructionDependency{
							{
								BuildId:  "8000",
								StepName: "compile",
							},
						},
					},
				},
			}

			updateMask := &field_mask.FieldMask{
				Paths: []string{"deadline", "bigquery_exports", "properties", "is_source_spec_final", "source_spec", "baseline_id", "realm", "test_instruction", "step_instructions"},
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
					TestInstruction:   testInstruction,
					StepInstructions:  stepInstructions,
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
				TestInstruction:   testInstruction,
				StepInstructions:  stepInstructions,
			}
			So(inv.Name, ShouldEqual, expected.Name)
			So(inv.State, ShouldEqual, pb.Invocation_ACTIVE)
			So(inv.Deadline, ShouldResembleProto, expected.Deadline)
			So(inv.Properties, ShouldResembleProto, expected.Properties)
			So(inv.SourceSpec, ShouldResembleProto, expected.SourceSpec)
			So(inv.IsSourceSpecFinal, ShouldEqual, expected.IsSourceSpecFinal)
			So(inv.BaselineId, ShouldEqual, expected.BaselineId)
			So(inv.Realm, ShouldEqual, expected.Realm)
			So(inv.TestInstruction, ShouldResembleProto, expected.TestInstruction)
			So(inv.StepInstructions, ShouldResembleProto, expected.StepInstructions)

			// Read from the database.
			actual := &pb.Invocation{
				Name:       expected.Name,
				SourceSpec: &pb.SourceSpec{},
			}
			invID := invocations.ID("inv")
			var compressedProperties spanutil.Compressed
			var compressedSources spanutil.Compressed
			var isSourceSpecFinal spanner.NullBool
			var compressedTestInstruction spanutil.Compressed
			var compressedStepInstructions spanutil.Compressed
			testutil.MustReadRow(ctx, "Invocations", invID.Key(), map[string]any{
				"Deadline":          &actual.Deadline,
				"BigQueryExports":   &actual.BigqueryExports,
				"Properties":        &compressedProperties,
				"Sources":           &compressedSources,
				"InheritSources":    &actual.SourceSpec.Inherit,
				"IsSourceSpecFinal": &isSourceSpecFinal,
				"BaselineId":        &actual.BaselineId,
				"Realm":             &actual.Realm,
				"TestInstruction":   &compressedTestInstruction,
				"StepInstructions":  &compressedStepInstructions,
			})
			actual.Properties = &structpb.Struct{}
			err = proto.Unmarshal(compressedProperties, actual.Properties)
			So(err, ShouldBeNil)
			actual.SourceSpec.Sources = &pb.Sources{}
			err = proto.Unmarshal(compressedSources, actual.SourceSpec.Sources)
			So(err, ShouldBeNil)
			actual.TestInstruction = &pb.Instruction{}
			err = proto.Unmarshal(compressedTestInstruction, actual.TestInstruction)
			So(err, ShouldBeNil)
			actual.StepInstructions = &pb.Instructions{}
			err = proto.Unmarshal(compressedStepInstructions, actual.StepInstructions)
			So(err, ShouldBeNil)
			if isSourceSpecFinal.Valid && isSourceSpecFinal.Bool {
				actual.IsSourceSpecFinal = true
			}
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
