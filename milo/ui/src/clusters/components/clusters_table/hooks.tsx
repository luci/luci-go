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

import { useCallback, useMemo } from 'react';

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
        if (failureFilter === '') {
          params.delete('q');
        } else {
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
  const interval: TimeInterval | undefined = useMemo(() => {
    if (intervalParam) {
      return intervals.find((option) => option.id === intervalParam);
    }
    return undefined;
  }, [intervalParam, intervals]);

  const updateIntervalParam = useCallback(
    (selectedInterval: TimeInterval, replace = false) => {
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
    },
    [setSearchParams],
  );

  return [interval, updateIntervalParam];
}

export function useOrderByParam(
  metrics: readonly ProjectMetric[],
): [OrderBy | undefined, (orderBy: OrderBy, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam = searchParams.get('orderBy') || '';
  const orderDir = searchParams.get('orderDir') || '';

  const orderBy: OrderBy | undefined = useMemo(() => {
    if (
      orderByParam &&
      metrics.some((metric) => metric.metricId === orderByParam)
    ) {
      return {
        metric: orderByParam,
        isAscending: orderDir === 'asc',
      };
    }
    return undefined;
  }, [metrics, orderByParam, orderDir]);

  const updateOrderByParams = useCallback(
    (orderBy: OrderBy, replace = false) => {
      setSearchParams(
        (stateParams) => {
          const params = new URLSearchParams(stateParams);
          if (orderBy) {
            params.set('orderBy', orderBy.metric);
            if (orderBy.isAscending) {
              params.set('orderDir', 'asc');
            }
          } else {
            params.delete('orderBy');
            params.delete('orderDir');
          }
          return params;
        },
        {
          replace,
        },
      );
    },
    [setSearchParams],
  );

  return [orderBy, updateOrderByParams];
}

export function useSelectedMetricsParam(
  metrics: readonly ProjectMetric[],
): [
  ProjectMetric[],
  (selectedMetrics: ProjectMetric[], replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';

  const selectedMetrics = useMemo(() => {
    const selectedMetricsIds = selectedMetricsParam.split(',');
    return metrics.filter(
      (metric) => selectedMetricsIds.indexOf(metric.metricId) > -1,
    );
  }, [metrics, selectedMetricsParam]);

  const updateSelectedMetricsParam = useCallback(
    (selectedMetrics: ProjectMetric[], replace = false) => {
      setSearchParams(
        (stateParams) => {
          const params = new URLSearchParams(stateParams);

          const selectedMetricsIds = selectedMetrics
            .map((metric) => metric.metricId)
            .join(',');
          params.set('selectedMetrics', selectedMetricsIds);

          const orderByParam = params.get('orderBy');
          if (selectedMetrics.every((m) => m.metricId !== orderByParam)) {
            const orderByMetric = selectedMetrics.reduce(
              (prev: ProjectMetric | null, m) => {
                if (!prev) {
                  return m;
                }
                return m.sortPriority > prev.sortPriority ? m : prev;
              },
              null,
            );
            const orderByValue = orderByMetric?.metricId || '';
            params.set('orderBy', orderByValue);
            params.set('orderDir', 'desc');
          }
          return params;
        },
        {
          replace,
        },
      );
    },
    [setSearchParams],
  );

  return [selectedMetrics, updateSelectedMetricsParam];
}
