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

// Package instructionutil contains utility functions for instructions.
package instructionutil

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/invocations"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// FilterInstructionType only keeps instructions of specific type.
// If the result instruction is empty after filter, the function will return nil.
func FilterInstructionType(instructions *pb.Instructions, instructionType pb.InstructionType) *pb.Instructions {
	if instructions == nil {
		return nil
	}
	filtered := []*pb.Instruction{}
	for _, instruction := range instructions.GetInstructions() {
		if instruction.Type == instructionType {
			filtered = append(filtered, instruction)
		}
	}

	if len(filtered) == 0 {
		return nil
	}
	return &pb.Instructions{
		Instructions: filtered,
	}
}

// RemoveInstructionsContent removes all contents from instructions.
// This is to reduce the memory footprint.
func RemoveInstructionsContent(instructions *pb.Instructions) *pb.Instructions {
	if instructions == nil {
		return nil
	}
	result := proto.Clone(instructions).(*pb.Instructions)
	for _, instruction := range result.GetInstructions() {
		for _, targetedInstruction := range instruction.GetTargetedInstructions() {
			targetedInstruction.Content = ""
		}
	}
	return result
}

func InstructionName(invocationID invocations.ID, instructionID string) string {
	return fmt.Sprintf("invocations/%s/instructions/%s", invocationID, instructionID)
}
