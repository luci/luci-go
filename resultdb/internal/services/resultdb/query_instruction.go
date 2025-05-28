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
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func validateQueryInstructionRequest(req *pb.QueryInstructionRequest) (invID invocations.ID, instructionID string, err error) {
	invocationID, instructionID, err := pbutil.ParseInstructionName(req.Name)
	if err != nil {
		return "", "", errors.Fmt("name: %w", err)
	}
	return invocations.ID(invocationID), instructionID, nil
}

func adjustMaxDepth(maxDepth int) int {
	if maxDepth <= 0 {
		return 5
	}
	if maxDepth > 10 {
		return 10
	}
	return maxDepth
}

func verifyQueryInstructionPermission(ctx context.Context, invID invocations.ID) error {
	return permissions.VerifyInvocation(ctx, invocations.ID(invID), rdbperms.PermGetInstruction)
}

// GetInstruction implements pb.ResultDBServer.
func (s *resultDBServer) QueryInstruction(ctx context.Context, req *pb.QueryInstructionRequest) (*pb.QueryInstructionResponse, error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	invocationID, instructionID, err := validateQueryInstructionRequest(req)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	maxDepth := adjustMaxDepth(int(req.DependencyMaxDepth))

	if err := verifyQueryInstructionPermission(ctx, invocationID); err != nil {
		return nil, err
	}

	instruction, err := fetchInstruction(ctx, invocationID, instructionID)
	if err != nil {
		return nil, err
	}

	dependencyChains := []*pb.InstructionDependencyChain{}
	for _, targetedInstruction := range instruction.TargetedInstructions {
		for _, target := range targetedInstruction.Targets {
			dependencyNodes, err := fetchDependencyNodes(ctx, invocationID, instruction, targetedInstruction, target, maxDepth)
			if err != nil {
				return nil, err
			}
			dependencyChains = append(dependencyChains, &pb.InstructionDependencyChain{
				Target: target,
				Nodes:  dependencyNodes,
			})
		}
	}

	return &pb.QueryInstructionResponse{
		Instruction:      instruction,
		DependencyChains: dependencyChains,
	}, nil
}

// fetchDependencyNodes fetches the dependency chain for an instruction.
func fetchDependencyNodes(ctx context.Context, invID invocations.ID, rootInstruction *pb.Instruction, ti *pb.TargetedInstruction, target pb.InstructionTarget, maxDepth int) ([]*pb.InstructionDependencyChain_Node, error) {
	targetedInstruction := ti
	// To store the instruction name that we processed.
	// Of the form {"instruction_name": "descriptive_name"}
	// This is to detect dependency cycle.
	processedInstructionName := map[string]string{}
	processedInstructionName[instructionutil.InstructionName(string(invID), rootInstruction.Id)] = rootInstruction.DescriptiveName
	depth := 0
	results := []*pb.InstructionDependencyChain_Node{}
	for depth < maxDepth {
		dependencies := targetedInstruction.Dependencies
		// No more dependency. Stop.
		if len(dependencies) == 0 {
			break
		}
		// We only support at most 1 dependency.
		dependency := dependencies[0]
		instructionName := instructionutil.InstructionName(dependency.InvocationId, dependency.InstructionId)
		// Check for permission.
		err := permissions.VerifyInvocation(ctx, invocations.ID(dependency.InvocationId), rdbperms.PermGetInstruction)
		// The user does not have permission to access the instruction for this dependency.
		if err != nil {
			st, ok := appstatus.Get(err)
			errMessage := ""
			if !ok {
				errMessage = err.Error()
			} else {
				errMessage = st.Message()
			}
			node := &pb.InstructionDependencyChain_Node{
				InstructionName: instructionName,
				Error:           errMessage,
			}
			results = append(results, node)
			break
		}

		// Check for dependency cycle.
		if descriptiveName, ok := processedInstructionName[instructionName]; ok {
			node := &pb.InstructionDependencyChain_Node{
				InstructionName: instructionName,
				Error:           "dependency cycle detected",
				DescriptiveName: descriptiveName,
			}
			results = append(results, node)
			break
		}

		// Query for the next targeted instruction.
		instruction, dependedInstruction, err := fetchTargetedInstruction(ctx, invocations.ID(dependency.InvocationId), dependency.InstructionId, target)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				node := &pb.InstructionDependencyChain_Node{
					InstructionName: instructionName,
					Error:           "targeted instruction not found",
				}
				results = append(results, node)
				break
			}
			// If this is another error, we will return to the caller.
			return nil, err
		}

		// We only allow dependency on step instruction.
		if instruction.Type != pb.InstructionType_STEP_INSTRUCTION {
			results = append(results, &pb.InstructionDependencyChain_Node{
				InstructionName: instructionName,
				Error:           "an instruction can only depend on a step instruction",
				DescriptiveName: instruction.GetDescriptiveName(),
			})
			break
		}

		// Found targeted instruction, create a new node.
		node := &pb.InstructionDependencyChain_Node{
			InstructionName: instructionName,
			Content:         dependedInstruction.Content,
			DescriptiveName: instruction.GetDescriptiveName(),
		}
		processedInstructionName[instructionName] = instruction.GetDescriptiveName()
		results = append(results, node)
		targetedInstruction = dependedInstruction
		depth++
	}
	return results, nil
}

func fetchTargetedInstruction(ctx context.Context, invID invocations.ID, instructionID string, target pb.InstructionTarget) (*pb.Instruction, *pb.TargetedInstruction, error) {
	instruction, err := fetchInstruction(ctx, invID, instructionID)
	if err != nil {
		return nil, nil, err
	}
	for _, targetedInstruction := range instruction.GetTargetedInstructions() {
		for _, t := range targetedInstruction.GetTargets() {
			if t == target {
				return instruction, targetedInstruction, nil
			}
		}
	}
	return nil, nil, appstatus.Errorf(codes.NotFound, "targeted instruction not found")
}
