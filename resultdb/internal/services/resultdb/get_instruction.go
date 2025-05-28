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
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func validateGetInstructionRequest(req *pb.GetInstructionRequest) (invocations.ID, string, error) {
	invocationID, instructionID, err := pbutil.ParseInstructionName(req.Name)
	if err != nil {
		return "", "", errors.Fmt("parse instruction name: %w", err)
	}
	return invocations.ID(invocationID), instructionID, nil
}

func verifyGetInstructionPermission(ctx context.Context, invID invocations.ID) error {
	return permissions.VerifyInvocation(ctx, invocations.ID(invID), rdbperms.PermGetInstruction)
}

// GetInstruction implements pb.ResultDBServer.
func (s *resultDBServer) GetInstruction(ctx context.Context, req *pb.GetInstructionRequest) (*pb.Instruction, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	invocationID, instructionID, err := validateGetInstructionRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	if err := verifyGetInstructionPermission(ctx, invocationID); err != nil {
		return nil, err
	}

	instruction, err := fetchInstruction(ctx, invocationID, instructionID)
	if err != nil {
		return nil, errors.Fmt("get instruction: %w", err)
	}

	return instruction, nil
}

func fetchInstruction(ctx context.Context, invID invocations.ID, instructionID string) (*pb.Instruction, error) {
	inv, err := invocations.Read(ctx, invID, invocations.ExcludeExtendedProperties)
	if err != nil {
		return nil, err
	}
	instructions := inv.GetInstructions()
	if instructions == nil {
		return nil, appstatus.Errorf(codes.NotFound, "instruction not found")
	}
	for _, instruction := range instructions.GetInstructions() {
		if instruction.Id == instructionID {
			return instruction, nil
		}
	}
	return nil, appstatus.Errorf(codes.NotFound, "instruction not found")
}
