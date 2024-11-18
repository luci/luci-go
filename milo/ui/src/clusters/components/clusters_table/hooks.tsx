// Copyright 2023 The LUCI Authors.
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

import { TimeInterval } from '@/clusters/hooks/use_fetch_clusters';
import { MetricId } from '@/clusters/types/metric_id';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

export interface OrderBy {
  metric: MetricId;
  isAscending: boolean;
}

export function useFilterParam(): [
  string,
  (failureFilter: string, replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const failureFilter = searchParams.get('q') || '';

  function updateFailureFilterParam(failureFilter: string, replace = false) {
    setSearchParams(
      (stateParams) => {
        const params = new URLSearchParams(stateParams);
        if (failureFilter !== '') {
          params.set('q', failureFilter);
        }
        return params;
      },
      {
        replace,
      },
    );
  }

  return [failureFilter, updateFailureFilterParam];
}

export function useIntervalParam(
  intervals: TimeInterval[],
): [
  TimeInterval | undefined,
  (selectedInterval: TimeInterval, replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const intervalParam = searchParams.get('interval') || '';
  let interval: TimeInterval | undefined = undefined;
  if (intervalParam) {
    interval = intervals.find((option) => option.id === intervalParam);
  }

  function updateIntervalParam(
    selectedInterval: TimeInterval,
    replace = false,
  ) {
    setSearchParams(
      (stateParams) => {
        const params = new URLSearchParams(stateParams);
        params.set('interval', selectedInterval.id);
        return params;
      },
      {
        replace,
      },
    );
  }

  return [interval, updateIntervalParam];
}

export function useOrderByParam(
  metrics: ProjectMetric[],
): [OrderBy | undefined, (orderBy: OrderBy, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam = searchParams.get('orderBy') || '';
  const orderDir = searchParams.get('orderDir') || '';

  let orderBy: OrderBy | undefined = undefined;

  if (orderByParam) {
    // Ensure the metric we are being asked to order by
    // is one of the metrics we are querying.
    if (metrics.some((metric) => metric.metricId === orderByParam)) {
      orderBy = {
        metric: orderByParam,
        isAscending: orderDir === 'asc',
      };
    }
  }

  function updateOrderByParams(orderBy: OrderBy, replace = false) {
    setSearchParams(
      (stateParams) => {
        const params = new URLSearchParams(stateParams);

        for (const [k, v] of searchParams.entries()) {
          if (k !== 'orderBy' && k !== 'orderDir') {
            params.set(k, v);
          }
        }
        if (orderBy) {
          params.set('orderBy', orderBy.metric);
          if (orderBy.isAscending) {
            params.set('orderDir', 'asc');
          }
        }
        return params;
      },
      {
        replace,
      },
    );
  }

  return [orderBy, updateOrderByParams];
}

export function useSelectedMetricsParam(
  metrics: ProjectMetric[],
): [
  ProjectMetric[],
  (selectedMetrics: ProjectMetric[], replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';
  const selectedMetricsIds = selectedMetricsParam.split(',');

  const selectedMetrics = metrics.filter(
    (metric) => selectedMetricsIds.indexOf(metric.metricId) > -1,
  );

  function updateSelectedMetricsParam(
    selectedMetrics: ProjectMetric[],
    replace = false,
  ) {
    setSearchParams(
      (stateParams) => {
        const params = new URLSearchParams(stateParams);

        const selectedMetricsIds = selectedMetrics
          .map((metric) => metric.metricId)
          .join(',');
        params.set('selectedMetrics', selectedMetricsIds);

        const orderByParam = searchParams.get('orderBy');
        if (selectedMetrics.findIndex((m) => m.metricId === orderByParam) < 0) {
          let orderByValue = '';
          if (selectedMetrics.length > 0) {
            let highestMetric = selectedMetrics[0];
            selectedMetrics.forEach((m) => {
              if (m.sortPriority > highestMetric.sortPriority) {
                highestMetric = m;
              }
            });
            orderByValue = highestMetric.metricId;
          }
          params.set('orderBy', orderByValue);
          params.set('orderDir', 'desc');
        }
        return params;
      },
      {
        replace,
      },
    );
  }

  return [selectedMetrics, updateSelectedMetricsParam];
}
