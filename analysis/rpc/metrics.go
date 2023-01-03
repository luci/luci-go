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

// List lists the metrics supported by the LUCI Analysis instance. See proto definition for more.
func (*metricsServer) List(ctx context.Context, req *pb.ListMetricsRequest) (*pb.ListMetricsResponse, error) {
	result := &pb.ListMetricsResponse{}
	for _, m := range metrics.ComputedMetrics {
		result.Metrics = append(result.Metrics, toMetricPB(m))

	}
	return result, nil
}

func toMetricPB(metric metrics.Definition) *pb.Metric {
	return &pb.Metric{
		Name:              metric.Name,
		MetricId:          metric.ID.String(),
		HumanReadableName: metric.HumanReadableName,
		Description:       metric.Description,
		IsDefault:         metric.IsDefault,
		SortPriority:      int32(metric.SortPriority),
	}
}
