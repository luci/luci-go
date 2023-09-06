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

	response := createProjectConfigPB(project, cfg.Config)
	return response, nil
}

func createProjectConfigPB(project string, cfg *configpb.ProjectConfig) *pb.ProjectConfig {
	result := &pb.ProjectConfig{
		Name:          fmt.Sprintf("projects/%s/config", project),
		BugSystem:     pb.ProjectConfig_BugSystem(cfg.BugSystem),
		Monorail:      toLegacyMonorailProjectPB(cfg.Monorail),
		Buganizer:     toLegacyBuganizerProjectPB(cfg.Buganizer),
		BugManagement: toBugManagementPB(cfg.BugManagement),
	}
	return result
}

func toBugManagementPB(bm *configpb.BugManagement) *pb.BugManagement {
	if bm == nil {
		// Always set BugManagement struct to avoid clients needing to
		// do pointless nullness checks. Having an empty BugManagement struct
		// is semantically identical to having an unset BugManagement struct.
		return &pb.BugManagement{}
	}
	policies := make([]*pb.BugManagementPolicy, 0, len(bm.Policies))
	for _, policy := range bm.Policies {
		policies = append(policies, toBugManagementPolicyPB(policy))
	}
	return &pb.BugManagement{
		Policies: policies,
		Monorail: toMonorailProjectPB(bm.Monorail),
	}
}

func toBugManagementPolicyPB(policy *configpb.BugManagementPolicy) *pb.BugManagementPolicy {
	metrics := make([]*pb.BugManagementPolicy_Metric, 0, len(policy.Metrics))
	for _, m := range policy.Metrics {
		metrics = append(metrics, &pb.BugManagementPolicy_Metric{
			MetricId:              m.MetricId,
			ActivationThreshold:   toMetricThresholdPB(m.ActivationThreshold),
			DeactivationThreshold: toMetricThresholdPB(m.DeactivationThreshold),
		})
	}

	return &pb.BugManagementPolicy{
		Id:                policy.Id,
		HumanReadableName: policy.HumanReadableName,
		Priority:          pb.BuganizerPriority(policy.Priority),
		Metrics:           metrics,
		Explanation: &pb.BugManagementPolicy_Explanation{
			// Explanation should never be nil. Config validation enforces this.
			ProblemHtml: policy.Explanation.ProblemHtml,
			ActionHtml:  policy.Explanation.ActionHtml,
		},
	}
}

func toMonorailProjectPB(cfg *configpb.MonorailProject) *pb.MonorailProject {
	if cfg == nil {
		return nil
	}
	return &pb.MonorailProject{
		Project:       cfg.Project,
		DisplayPrefix: cfg.DisplayPrefix,
	}
}

func toLegacyMonorailProjectPB(cfg *configpb.MonorailProject) *pb.MonorailProject {
	if cfg == nil {
		return nil
	}
	// Similar to toMetricProjectPB, but converts legacy fields as well.
	thresholds := make([]*pb.MonorailPriority, 0, len(cfg.Priorities))
	for _, monorailPriorityThreshold := range cfg.Priorities {
		priorityThreshold := &pb.MonorailPriority{
			Priority:   monorailPriorityThreshold.Priority,
			Thresholds: toImpactMetricThresholdsPB(monorailPriorityThreshold.Thresholds),
		}
		thresholds = append(thresholds, priorityThreshold)
	}

	return &pb.MonorailProject{
		Project:       cfg.Project,
		DisplayPrefix: cfg.DisplayPrefix,
		Priorities:    thresholds,
	}
}

func toLegacyBuganizerProjectPB(cfg *configpb.BuganizerProject) *pb.BuganizerProject {
	if cfg == nil {
		return nil
	}
	thresholds := []*pb.BuganizerProject_PriorityMapping{}
	for _, p := range cfg.PriorityMappings {
		threshold := &pb.BuganizerProject_PriorityMapping{
			Priority:   pb.BuganizerPriority(p.Priority),
			Thresholds: toImpactMetricThresholdsPB(p.Thresholds),
		}
		thresholds = append(thresholds, threshold)
	}
	return &pb.BuganizerProject{
		PriorityMappings: thresholds,
	}
}

func toImpactMetricThresholdsPB(impactThresholds []*configpb.ImpactMetricThreshold) []*pb.ImpactMetricThreshold {
	result := make([]*pb.ImpactMetricThreshold, len(impactThresholds))
	for i, impactThreshold := range impactThresholds {
		result[i] = &pb.ImpactMetricThreshold{
			MetricId:  impactThreshold.MetricId,
			Threshold: toMetricThresholdPB(impactThreshold.Threshold),
		}
	}
	return result
}

func toMetricThresholdPB(threshold *configpb.MetricThreshold) *pb.MetricThreshold {
	if threshold == nil {
		return nil
	}
	return &pb.MetricThreshold{
		OneDay:   threshold.OneDay,
		ThreeDay: threshold.ThreeDay,
		SevenDay: threshold.SevenDay,
	}
}

func (*projectServer) List(ctx context.Context, request *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	readableRealms, err := auth.QueryRealms(ctx, perms.PermGetConfig, "", nil)
	if err != nil {
		return nil, err
	}
	readableProjects := make([]string, 0)
	for _, r := range readableRealms {
		project, realm := realms.Split(r)
		if realm == realms.RootRealm || realm == realms.ProjectRealm {
			readableProjects = append(readableProjects, project)
		}
	}
	// Return projects in a stable order.
	sort.Strings(readableProjects)

	return &pb.ListProjectsResponse{
		Projects: createProjectPBs(readableProjects),
	}, nil
}

func createProjectPBs(projects []string) []*pb.Project {
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
