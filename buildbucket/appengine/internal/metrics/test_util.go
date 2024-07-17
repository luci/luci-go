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

package metrics

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func getCustomMetricsAndState(ctx context.Context) (map[pb.CustomBuildMetricBase]map[string]CustomMetric, *tsmon.State) {
	cms := getCustomMetrics(ctx)
	cms.m.RLock()
	defer cms.m.RUnlock()
	return cms.metrics, cms.state
}

func GetCustomMetricsData(ctx context.Context, base pb.CustomBuildMetricBase, name string, resetTime time.Time, fvs []any) (any, error) {
	metrics, state := getCustomMetricsAndState(ctx)
	store := state.Store()
	cm, ok := metrics[base][name]
	if !ok {
		return nil, errors.Reason("metric with base %s, name %s not found", base, name).Err()
	}

	switch base {
	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CREATED,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_STARTED,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_COMPLETED:
		return store.Get(ctx, cm.(*counter), resetTime, fvs), nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CYCLE_DURATIONS,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_RUN_DURATIONS,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_SCHEDULING_DURATIONS:
		return store.Get(ctx, cm.(*cumulativeDistribution), resetTime, fvs), nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_MAX_AGE_SCHEDULED:
		return store.Get(ctx, cm.(*float), resetTime, fvs), nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_COUNT,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT:
		return store.Get(ctx, cm.(*int), resetTime, fvs), nil

	default:
		return nil, errors.Reason("invalid metric base %s", base).Err()
	}
}
