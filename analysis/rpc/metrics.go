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

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/perms"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type metricsServer struct {
}

func NewMetricsServer() *pb.DecoratedMetrics {
	return &pb.DecoratedMetrics{
		Prelude:  checkAllowedPrelude,
		Service:  &metricsServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// ListForProject lists the metrics for a given LUCI Project. See proto definition for more.
func (*metricsServer) ListForProject(ctx context.Context, req *pb.ListProjectMetricsRequest) (*pb.ListProjectMetricsResponse, error) {
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetConfig); err != nil {
		return nil, err
	}
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}
	result := &pb.ListProjectMetricsResponse{}
	for _, m := range metrics.ComputedMetrics {
		projectMetric := m.AdaptToProject(project, cfg.Config.Metrics)
		if projectMetric.Config.ShowInMetricsSelector {
			result.Metrics = append(result.Metrics, toProjectMetricPB(projectMetric))
		}
	}
	return result, nil
}

func toProjectMetricPB(metric metrics.Definition) *pb.ProjectMetric {
	return &pb.ProjectMetric{
		Name:              metric.Name,
		MetricId:          metric.ID.String(),
		HumanReadableName: metric.HumanReadableName,
		Description:       metric.Description,
		IsDefault:         metric.Config.IsDefault,
		SortPriority:      int32(metric.Config.SortPriority),
	}
}
