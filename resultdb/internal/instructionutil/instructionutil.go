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

// InstructionsWithNames populates the instructions with names.
func InstructionsWithNames(instructions *pb.Instructions, parentName string) *pb.Instructions {
	if instructions == nil {
		return nil
	}
	result := proto.Clone(instructions).(*pb.Instructions)
	for _, instruction := range result.GetInstructions() {
		instruction.Name = InstructionName(parentName, instruction.Id)
	}
	return result
}

// InstructionWithNames populates the instruction with names.
func InstructionWithNames(instruction *pb.Instruction, parentName string) *pb.Instruction {
	if instruction == nil {
		return nil
	}
	result := proto.Clone(instruction).(*pb.Instruction)
	result.Name = InstructionName(parentName, instruction.Id)
	return result
}

// InstructionName returns instruction name given parentName and instructionID.
// parentName is the name of the parent resource of the instruction
// i.e. an invocation name or a work unit name.
func InstructionName(parentName string, instructionID string) string {
	return fmt.Sprintf("%s/instructions/%s", parentName, instructionID)
}

// RemoveInstructionsName removes all names from instructions.
// This is to be used in create/update invocations.
// Name is an output-only field, we will not store it in Spanner.
func RemoveInstructionsName(instructions *pb.Instructions) *pb.Instructions {
	if instructions == nil {
		return nil
	}
	result := proto.Clone(instructions).(*pb.Instructions)
	for _, instruction := range result.GetInstructions() {
		instruction.Name = ""
	}
	return result
}
