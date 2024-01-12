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

import {
  ParamKeyValuePair,
  useSearchParams,
} from 'react-router-dom';

import { TimeInterval } from '@/hooks/use_fetch_clusters';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { MetricId } from '@/legacy_services/shared_models';

export interface OrderBy {
  metric: MetricId,
  isAscending: boolean,
}

export function useFilterParam(): [string, (failureFilter: string, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const failureFilter = searchParams.get('q') || '';

  function updateFailureFilterParam(failureFilter: string, replace = false) {
    const params: ParamKeyValuePair[] = [];

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'q') {
        params.push([k, v]);
      }
    }

    if (failureFilter !== '') {
      params.push(['q', failureFilter]);
    }
    setSearchParams(params, {
      replace,
    });
  }

  return [failureFilter, updateFailureFilterParam];
}

export function useIntervalParam(intervals: TimeInterval[]): [TimeInterval | undefined, (selectedInterval: TimeInterval, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const intervalParam = searchParams.get('interval') || '';
  let interval: TimeInterval | undefined = undefined;
  if (intervalParam) {
    interval = intervals.find((option) => option.id === intervalParam);
  }

  function updateIntervalParam(selectedInterval: TimeInterval, replace = false) {
    const params: ParamKeyValuePair[] = [];

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'interval') {
        params.push([k, v]);
      }
    }

    params.push(['interval', selectedInterval.id]);

    setSearchParams(params, {
      replace,
    });
  }

  return [interval, updateIntervalParam];
}

export function useOrderByParam(metrics: ProjectMetric[]): [OrderBy | undefined, (orderBy: OrderBy, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const orderByParam = searchParams.get('orderBy') || '';
  const orderDir = searchParams.get('orderDir') || '';

  let orderBy: OrderBy | undefined = undefined;

  if (orderByParam) {
    // Ensure the metric we are being asked to order by
    // is one of the metrics we are querying.
    if (metrics.some((metric) => metric.metricId == orderByParam)) {
      orderBy = {
        metric: orderByParam,
        isAscending: orderDir === 'asc',
      };
    }
  }

  function updateOrderByParams(orderBy: OrderBy, replace = false) {
    const params: ParamKeyValuePair[] = [];

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'orderBy' && k !== 'orderDir') {
        params.push([k, v]);
      }
    }
    if (orderBy) {
      params.push(['orderBy', orderBy.metric]);
      if (orderBy.isAscending) {
        params.push(['orderDir', 'asc']);
      }
    }
    setSearchParams(params, {
      replace,
    });
  }

  return [orderBy, updateOrderByParams];
}

export function useSelectedMetricsParam(metrics: ProjectMetric[]): [ProjectMetric[], (selectedMetrics: ProjectMetric[], replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';
  const selectedMetricsIds = selectedMetricsParam.split(',');

  const selectedMetrics = metrics.filter((metric) => selectedMetricsIds.indexOf(metric.metricId) > -1);

  function updateSelectedMetricsParam(selectedMetrics: ProjectMetric[], replace = false) {
    const params: ParamKeyValuePair[] = [];

    const selectedMetricsIds = selectedMetrics.map((metric) => metric.metricId).join(',');
    params.push(['selectedMetrics', selectedMetricsIds]);

    const orderByParam = searchParams.get('orderBy');
    let addedOrderBy = false;
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
      params.push(['orderBy', orderByValue]);
      params.push(['orderDir', 'desc']);
      addedOrderBy = true;
    }

    for (const [k, v] of searchParams.entries()) {
      if (((k === 'orderBy' || k === 'orderDir') && addedOrderBy) ||
        k === 'selectedMetrics') {
        continue;
      }
      params.push([k, v]);
    }
    setSearchParams(params, {
      replace,
    });
  }

  return [selectedMetrics, updateSelectedMetricsParam];
}
