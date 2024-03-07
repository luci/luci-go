// Copyright 2022 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/proto"
)

// Check if a step is compile step.
// For the moment, we only check for the step name.
// In the future, we may want to check for step tag (crbug.com/1353978)
func IsCompileStep(step *bbpb.Step) bool {
	return step.GetName() == "compile"
}

// GetGitilesCommitForBuild returns the gitiles commit for a build
// The commit position is taken from output properties if available.
func GetGitilesCommitForBuild(build *bbpb.Build) *bbpb.GitilesCommit {
	commit := &bbpb.GitilesCommit{}
	input := build.GetInput()
	if input != nil && input.GitilesCommit != nil {
		proto.Merge(commit, input.GitilesCommit)
	}
	// Commit position only exists in output, not in input
	output := build.GetOutput()
	if output != nil && output.GitilesCommit != nil {
		// Just do a sanity check here
		outputCommit := output.GitilesCommit
		if outputCommit.Host == commit.Host && outputCommit.Project == commit.Project && outputCommit.Id == commit.Id && outputCommit.Ref == commit.Ref {
			commit.Position = outputCommit.Position
		}
	}
	return commit
}

func ToDimensionsPB(dimensions []*bbpb.RequestedDimension) *pb.Dimensions {
	dims := []*pb.Dimension{}
	for _, d := range dimensions {
		dims = append(dims, &pb.Dimension{
			Key:   d.Key,
			Value: d.Value,
		})
	}
	return &pb.Dimensions{Dimensions: dims}
}

// GetSheriffRotationsForBuild returns the sheriff rotations for a build.
// If there is no sheriff rotation set, return empty list.
func GetSheriffRotationsForBuild(build *bbpb.Build) []string {
	fields := build.GetInput().GetProperties().GetFields()
	value, found := fields["sheriff_rotations"]
	if !found {
		return []string{}
	}
	listValue := value.GetListValue()
	if listValue == nil {
		return []string{}
	}
	var rotations []string
	for _, value := range listValue.Values {
		rotation := value.GetStringValue()
		if rotation != "" {
			rotations = append(rotations, rotation)
		}
	}
	return rotations
}

func BuilderFromConfigBuilder(builder *configpb.Builder) *bbpb.BuilderID {
	return &bbpb.BuilderID{
		Project: builder.Project,
		Bucket:  builder.Bucket,
		Builder: builder.Builder,
	}
}

// GetBuilderGroup returns the builder group for a build.
// If there is no builder group set, return empty string.
func GetBuilderGroup(build *bbpb.Build) string {
	fields := build.GetInput().GetProperties().GetFields()
	value, found := fields["builder_group"]
	if !found {
		return ""
	}
	return value.GetStringValue()
}

func GetTaskDimensions(build *bbpb.Build) []*bbpb.RequestedDimension {
	if build.GetInfra().GetSwarming().GetTaskDimensions() != nil {
		return build.GetInfra().GetSwarming().GetTaskDimensions()
	}
	return build.GetInfra().GetBackend().GetTaskDimensions()
}
