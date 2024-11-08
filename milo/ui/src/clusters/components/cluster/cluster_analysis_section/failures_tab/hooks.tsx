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

import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { MetricName } from '@/clusters/tools/failures_tools';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

export function useSelectedVariantGroupsParam(): [
  string[],
  (selectedVariantGroups: string[], replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const groupByParam = searchParams.get('groupBy') || '';
  let selectedVariantGroups: string[] = [];
  if (groupByParam) {
    selectedVariantGroups = groupByParam.split(',');
  }

  function updateSelectedVariantGroupsParam(
    selectedVariantGroups: string[],
    replace = false,
  ) {
    const params = new URLSearchParams();

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'groupBy') {
        params.set(k, v);
      }
    }

    params.set('groupBy', selectedVariantGroups.join(','));

    setSearchParams(params, {
      replace,
    });
  }

  return [selectedVariantGroups, updateSelectedVariantGroupsParam];
}

export function useFilterToMetricParam(
  metrics: ProjectMetric[],
): [
  ProjectMetric | undefined,
  (filterToMetric: ProjectMetric | undefined, replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const filterToMetricParam = searchParams.get('filterToMetric') || '';

  const filterToMetric = metrics.find(
    (metric) => filterToMetricParam === metric.metricId,
  );

  function updateFilterToMetricParam(
    filterToMetric: ProjectMetric | undefined,
    replace = false,
  ) {
    const params = new URLSearchParams();
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'filterToMetric') {
        params.set(k, v);
      }
    }

    if (filterToMetric) {
      params.set('filterToMetric', filterToMetric.metricId);
    }

    setSearchParams(params, {
      replace,
    });
  }

  return [filterToMetric, updateFilterToMetricParam];
}

export function useOrderByParam(
  defaultMetricName: MetricName,
): [MetricName, (metricName: MetricName, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const orderByParam =
    (searchParams.get('orderBy') as MetricName) || defaultMetricName;

  function updateOrderByParam(metricName: MetricName, replace = false) {
    const params = new URLSearchParams();
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'orderBy') {
        params.set(k, v);
      }
    }

    params.set('orderBy', metricName);

    setSearchParams(params, {
      replace,
    });
  }

  return [orderByParam, updateOrderByParam];
}
