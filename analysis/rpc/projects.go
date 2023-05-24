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

package rpc

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/perms"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type projectServer struct{}

func NewProjectsServer() *pb.DecoratedProjects {
	return &pb.DecoratedProjects{
		Prelude:  checkAllowedPrelude,
		Service:  &projectServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

func (*projectServer) GetConfig(ctx context.Context, req *pb.GetProjectConfigRequest) (*pb.ProjectConfig, error) {
	project, err := parseProjectConfigName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetConfig); err != nil {
		return nil, err
	}

	// Fetch a recent project configuration.
	// (May be a recent value that was cached.)
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	response := &pb.ProjectConfig{
		Name:      fmt.Sprintf("projects/%s/config", project),
		BugSystem: pb.ProjectConfig_BugSystem(cfg.Config.BugSystem),
	}
	if cfg.Config.Monorail != nil {
		response.Monorail = &pb.MonorailProject{
			Project:       cfg.Config.Monorail.Project,
			DisplayPrefix: cfg.Config.Monorail.DisplayPrefix,
		}

		thresholds := []*pb.MonorailPriority{}
		for _, monorailPriorityThreshold := range cfg.Config.Monorail.GetPriorities() {
			priorityThreshold := &pb.MonorailPriority{
				Priority: monorailPriorityThreshold.GetPriority(),
				Thresholds: convertImpactMetricThresholds(
					monorailPriorityThreshold.GetThresholds(),
				),
			}
			thresholds = append(thresholds, priorityThreshold)
		}
		response.Monorail.Priorities = thresholds
	}

	if cfg.Config.Buganizer != nil {
		thresholds := []*pb.BuganizerProject_PriorityMapping{}
		buganizerPriorities := cfg.Config.Buganizer.GetPriorityMappings()
		for _, buganizerPriorityThreshold := range buganizerPriorities {
			priorityThreshold := &pb.BuganizerProject_PriorityMapping{
				Priority: pb.BuganizerPriority(buganizerPriorityThreshold.GetPriority()),
				Thresholds: convertImpactMetricThresholds(
					buganizerPriorityThreshold.GetThresholds(),
				),
			}
			thresholds = append(thresholds, priorityThreshold)
		}
		response.Buganizer = &pb.BuganizerProject{
			PriorityMappings: thresholds,
		}
	}

	return response, nil
}

func convertImpactMetricThresholds(impactThresholds []*configpb.ImpactMetricThreshold) []*pb.ImpactMetricThreshold {
	result := make([]*pb.ImpactMetricThreshold, len(impactThresholds))
	for i, impactThreshold := range impactThresholds {
		result[i] = &pb.ImpactMetricThreshold{
			MetricId: impactThreshold.MetricId,
			Threshold: &pb.MetricThreshold{
				OneDay:   impactThreshold.Threshold.OneDay,
				ThreeDay: impactThreshold.Threshold.ThreeDay,
				SevenDay: impactThreshold.Threshold.SevenDay,
			},
		}
	}

	return result
}

func (*projectServer) List(ctx context.Context, request *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	readableRealms, err := auth.QueryRealms(ctx, perms.PermGetConfig, "", nil)
	if err != nil {
		return nil, err
	}
	readableProjects := make([]string, 0)
	for _, r := range readableRealms {
		project, realm := realms.Split(r)
		if realm == "@root" {
			readableProjects = append(readableProjects, project)
		}
	}
	// Return projects in a stable order.
	sort.Strings(readableProjects)

	return &pb.ListProjectsResponse{
		Projects: createProjectPbs(readableProjects),
	}, nil
}

func createProjectPbs(projects []string) []*pb.Project {
	projectsPbs := make([]*pb.Project, 0, len(projects))
	for _, project := range projects {
		projectsPbs = append(projectsPbs, &pb.Project{
			Name:        fmt.Sprintf("projects/%s", project),
			DisplayName: strings.Title(project),
			Project:     project,
		})
	}
	return projectsPbs
}
