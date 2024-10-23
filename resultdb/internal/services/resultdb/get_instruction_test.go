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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateGetInstructionRequest(t *testing.T) {
	ftt.Run("ValidateGetInstructionRequest", t, func(t *ftt.Test) {
		t.Run("Empty name", func(t *ftt.Test) {
			req := &pb.GetInstructionRequest{}
			_, _, err := validateGetInstructionRequest(req)
			assert.Loosely(t, err, should.ErrLike("unspecified"))
		})
		t.Run("Invalid name", func(t *ftt.Test) {
			req := &pb.GetInstructionRequest{
				Name: "some invalid name",
			}
			_, _, err := validateGetInstructionRequest(req)
			assert.Loosely(t, err, should.ErrLike("does not match"))
		})
		t.Run("Instruction name", func(t *ftt.Test) {
			req := &pb.GetInstructionRequest{
				Name: "invocations/build-8888/instructions/a_simple_instruction",
			}
			invocationID, instructionID, err := validateGetInstructionRequest(req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invocationID, should.Equal(invocations.ID("build-8888")))
			assert.Loosely(t, instructionID, should.Equal("a_simple_instruction"))
		})
	})
}

func TestGetInstruction(t *testing.T) {
	ftt.Run(`GetInstruction`, t, func(t *ftt.Test) {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermGetInstruction},
			},
		})
		srv := newTestResultDBService()

		t.Run(`Permission denied`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{"Realm": "secretproject:testrealm"}),
			)
			req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/test_instruction"}
			_, err := srv.GetInstruction(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run(`Invalid name`, func(t *ftt.Test) {
			req := &pb.GetInstructionRequest{Name: "Some invalid name"}
			_, err := srv.GetInstruction(ctx, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
		})

		t.Run(`Get instruction`, func(t *ftt.Test) {
			// Insert an invocation.
			testutil.MustApply(ctx, t,
				insert.Invocation("build-12345", pb.Invocation_ACTIVE, map[string]any{
					"Realm": "testproject:testrealm",
					"Instructions": spanutil.Compress(pbutil.MustMarshal(&pb.Instructions{
						Instructions: []*pb.Instruction{
							{
								Id:              "my_instruction",
								DescriptiveName: "My Instruction",
								Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
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
			t.Run("Invocation not found", func(t *ftt.Test) {
				req := &pb.GetInstructionRequest{Name: "invocations/build-xxx/instructions/test_instruction"}
				_, err := srv.GetInstruction(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})

			t.Run("Instruction not found", func(t *ftt.Test) {
				req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/not_found"}
				_, err := srv.GetInstruction(ctx, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)())
			})

			t.Run("Instruction found", func(t *ftt.Test) {
				req := &pb.GetInstructionRequest{Name: "invocations/build-12345/instructions/my_instruction"}
				instruction, err := srv.GetInstruction(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, instruction, should.Resemble(&pb.Instruction{
					Id:              "my_instruction",
					Type:            pb.InstructionType_TEST_RESULT_INSTRUCTION,
					DescriptiveName: "My Instruction",
					Name:            "invocations/build-12345/instructions/my_instruction",
					TargetedInstructions: []*pb.TargetedInstruction{
						{
							Targets: []pb.InstructionTarget{
								pb.InstructionTarget_LOCAL,
							},
							Content: "this content",
						},
					},
				}))
			})
		})
	})
}
