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

import { useQuery, UseQueryResult } from 'react-query';

import {
  getClustersService,
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
} from '@/services/cluster';
import {
  prpcRetrier,
  MetricId,
} from '@/services/shared_models';
import {
  Metric,
} from '@/services/metrics';

export interface OrderBy {
  metric: MetricId,
  isAscending: boolean,
}

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
const metricsKey = (metrics: Metric[]): string => {
  const metricIds = metrics.map((m) => m.metricId);
  // Sort to ensure we treat the input as a set instead
  // of a list.
  metricIds.sort();
  // Metric IDs only contain characters in [a-z0-9-]
  // so it is safe to concatenate them with other characters
  // while still guaranteeing the returned keys is unique
  // for each combination of metrics.
  return metricIds.join(':');
};

const useFetchClusters = (
    project: string,
    failureFilter: string,
    orderBy: OrderBy | undefined,
    metrics: Metric[],
): UseQueryResult<QueryClusterSummariesResponse, Error> => {
  const clustersService = getClustersService();
  return useQuery(
      ['clusters', project, failureFilter, orderByClause(orderBy), metricsKey(metrics)],
      async () => {
        const request : QueryClusterSummariesRequest = {
          project: project,
          failureFilter: failureFilter,
          orderBy: orderByClause(orderBy),
          metrics: metrics.map((m) => m.name),
        };

        return await clustersService.queryClusterSummaries(request);
      }, {
        retry: prpcRetrier,
      },
  );
};

export default useFetchClusters;
