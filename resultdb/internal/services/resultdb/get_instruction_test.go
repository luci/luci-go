// Copyright 2024 The LUCI Authors.
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

package resultdb

import (
	"testing"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateGetInstructionRequest(t *testing.T) {
	Convey("ValidateGetInstructionRequest", t, func() {
		Convey("Empty name", func() {
			req := &pb.GetInstructionRequest{}
			_, _, err := validateGetInstructionRequest(req)
			So(err, ShouldErrLike, "unspecified")
		})
		Convey("Invalid name", func() {
			req := &pb.GetInstructionRequest{
				Name: "some invalid name",
			}
			_, _, err := validateGetInstructionRequest(req)
			So(err, ShouldErrLike, "does not match")
		})
		Convey("Instruction name", func() {
			req := &pb.GetInstructionRequest{
				Name: "invocations/build-8888/instructions/a_simple_instruction",
			}
			invocationID, instructionID, err := validateGetInstructionRequest(req)
			So(err, ShouldBeNil)
			So(invocationID, ShouldEqual, invocations.ID("build-8888"))
			So(instructionID, ShouldEqual, "a_simple_instruction")
		})
	})
}

func TestGetInstruction(t *testing.T) {
	Convey(`GetInstruction`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInstruction},
			},
		})
		srv := newTestResultDBService()

		Convey(`Permission denied`, func() {
			testutil.MustApply(ctx,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
			)
			req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/test_instruction"}
			_, err := srv.GetInstruction(ctx, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`Invalid name`, func() {
			req := &pb.GetInstructionRequest{Name: "Some invalid name"}
			_, err := srv.GetInstruction(ctx, req)
			So(err, ShouldBeRPCInvalidArgument)
		})

		Convey(`Get instruction`, func() {
			// Insert an invocation.
			testutil.MustApply(ctx,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "testproject:testrealm",
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
						Instructions: []*pb.Instruction{
							{
								Id:   "my_instruction",
								Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
								TargetedInstructions: []*pb.TargetedInstruction{
									{
										Targets: []pb.InstructionTarget{
											pb.InstructionTarget_LOCAL,
										},
										Content: "this content",
									},
								},
							},
						},
					})),
				}),
			)
			Convey("Invocation not found", func() {
				req := &pb.GetInstructionRequest{Name: "invocations/build-xxx/instructions/test_instruction"}
				_, err := srv.GetInstruction(ctx, req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("Instruction not found", func() {
				req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/not_found"}
				_, err := srv.GetInstruction(ctx, req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("Instruction found", func() {
				req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/my_instruction"}
				instruction, err := srv.GetInstruction(ctx, req)
				So(err, ShouldBeNil)
				So(instruction, ShouldResembleProto, &pb.Instruction{
					Id:   "my_instruction",
					Type: pb.InstructionType_TEST_RESULT_INSTRUCTION,
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "this content",
						},
					},
				})
			})
		})
	})
}
