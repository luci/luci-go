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

import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { DateTime } from 'luxon';

import { useClustersService } from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import { MetricId } from '@/clusters/types/metric_id';
import {
  ClusterSummaryView,
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

export interface ClustersFetchOptions {
  project: string;
  failureFilter: string;
  orderBy?: OrderBy;
  metrics: ProjectMetric[];
  interval?: TimeInterval;
}

export interface OrderBy {
  metric: MetricId;
  isAscending: boolean;
}

export interface TimeInterval {
  id: string; // ID for the time interval, e.g. '3d'
  label: string; // Human-readable name for the time interval, e.g. '3 days'
  duration: number; // Duration of the time interval in hours
}

const intervalDuration = (interval?: TimeInterval): number => {
  if (!interval) {
    return 0;
  }
  return interval.duration;
};

// orderByClause returns the AIP-132 order by clause needed
// to sort by the given metric.
const orderByClause = (orderBy?: OrderBy): string => {
  if (!orderBy) {
    return '';
  }
  return `metrics.\`${orderBy.metric}\`.value${orderBy.isAscending ? '' : ' desc'}`;
};

// metricsKey returns a unique key to represent the given
// set of metrics.
export const metricsKey = (metrics: ProjectMetric[]): string => {
  const metricNames = metrics.map((m) => m.name);
  // Sort to ensure we treat the input as a set instead
  // of a list.
  metricNames.sort();
  // Metric IDs only contain characters in [a-z0-9-]
  // so it is safe to concatenate them with other characters
  // while still guaranteeing the returned keys is unique
  // for each combination of metrics.
  return metricNames.join(':');
};

export const useFetchClusterSummaries = (
  { project, failureFilter, orderBy, interval, metrics }: ClustersFetchOptions,
  view: ClusterSummaryView,
): UseQueryResult<QueryClusterSummariesResponse, Error> => {
  const clustersService = useClustersService();
  return useQuery({
    queryKey: [
      'clusters',
      view,
      project,
      failureFilter,
      orderByClause(orderBy),
      intervalDuration(interval),
      metricsKey(metrics),
    ],

    queryFn: async () => {
      const latestTime = DateTime.now();

      const request: QueryClusterSummariesRequest = {
        project: project,
        timeRange: {
          earliest: latestTime
            .minus({
              hour: intervalDuration(interval),
            })
            .toISO(),
          latest: latestTime.toISO(),
        },
        failureFilter: failureFilter,
        orderBy: orderByClause(orderBy),
        metrics: metrics.map((m) => m.name),
        view: view,
      };
      return await clustersService.QueryClusterSummaries(request);
    },

    retry: prpcRetrier,

    enabled:
      orderBy !== undefined &&
      orderBy.metric !== '' &&
      metrics.length > 0 &&
      interval !== undefined &&
      (view !== ClusterSummaryView.FULL ||
        (ClusterSummaryView.FULL && interval.duration > 24)),
  });
};
