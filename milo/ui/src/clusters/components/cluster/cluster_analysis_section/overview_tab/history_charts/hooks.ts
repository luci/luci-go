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

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

export interface HistoryTimeRange {
  id: string;
  label: string;
  value: number;
}

export function useAnnotatedParam(): [
  boolean | undefined,
  (isAnnotated: boolean, replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const annotatedParam = searchParams.get('annotated') || '';
  const annotatedParamLower = annotatedParam.toLowerCase();

  const annotated: boolean | undefined = useMemo(() => {
    if (annotatedParamLower === 'true') {
      return true;
    } else if (annotatedParamLower === 'false') {
      return false;
    }
    return undefined;
  }, [annotatedParamLower]);

  const updateAnnotatedParam = useCallback(
    (newAnnotated: boolean, replace = false) => {
      setSearchParams(
        (params) => {
          const newParams = new URLSearchParams(params);
          newParams.set('annotated', newAnnotated ? 'true' : 'false');
          return newParams;
        },
        {
          replace,
        },
      );
    },
    [setSearchParams],
  );

  return [annotated, updateAnnotatedParam];
}

export function useHistoryTimeRangeParam(
  options: HistoryTimeRange[],
): [
  HistoryTimeRange | undefined,
  (selectedHistoryTimeRange: HistoryTimeRange, replace?: boolean) => void,
] {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const timeRangeParam = searchParams.get('historyTimeRange') || '';
  const timeRange: HistoryTimeRange | undefined = useMemo(() => {
    if (timeRangeParam) {
      return options.find((option) => option.id === timeRangeParam);
    }
    return undefined;
  }, [options, timeRangeParam]);

  const updateHistoryTimeRangeParam = useCallback(
    (selectedHistoryTimeRange: HistoryTimeRange, replace = false) => {
      setSearchParams(
        (params) => {
          const newParams = new URLSearchParams(params);
          newParams.set('historyTimeRange', selectedHistoryTimeRange.id);
          return newParams;
        },
        {
          replace,
        },
      );
    },
    [setSearchParams],
  );

  return [timeRange, updateHistoryTimeRangeParam];
}

export function useSelectedMetricsParam(
  metrics: ProjectMetric[],
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
      const selectedMetricsIds = selectedMetrics
        .map((metric) => metric.metricId)
        .join(',');

      setSearchParams(
        (params) => {
          const newParams = new URLSearchParams(params);
          newParams.set('selectedMetrics', selectedMetricsIds);
          return newParams;
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
