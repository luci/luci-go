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

import { Metric } from '@/legacy_services/metrics';

export interface HistoryTimeRange {
  id: string;
  label: string;
  value: number;
}

export function useAnnotatedParam(): [boolean | undefined, (isAnnotated: boolean, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  let annotatedParam = searchParams.get('annotated') || '';
  annotatedParam = annotatedParam.toLowerCase();
  let annotated: boolean | undefined = undefined;
  if (annotatedParam === 'true') {
    annotated = true;
  } else if (annotatedParam === 'false') {
    annotated = false;
  }

  function updateAnnotatedParam(newAnnotated: boolean, replace = false) {
    const params: ParamKeyValuePair[] = [];
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'annotated') {
        params.push([k, v]);
      }
    }

    params.push(['annotated', newAnnotated ? 'true' : 'false']);
    setSearchParams(params, {
      replace,
    });
  }

  return [annotated, updateAnnotatedParam];
}

export function useHistoryTimeRangeParam(options: HistoryTimeRange[]): [HistoryTimeRange | undefined, (selectedHistoryTimeRange: HistoryTimeRange, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const timeRangeParam = searchParams.get('historyTimeRange') || '';
  let timeRange: HistoryTimeRange | undefined = undefined;
  if (timeRangeParam) {
    timeRange = options.find((option) => option.id === timeRangeParam);
  }

  function updateHistoryTimeRangeParam(selectedHistoryTimeRange: HistoryTimeRange, replace = false) {
    const params: ParamKeyValuePair[] = [];
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'historyTimeRange') {
        params.push([k, v]);
      }
    }

    params.push(['historyTimeRange', selectedHistoryTimeRange.id]);
    setSearchParams(params, {
      replace,
    });
  }

  return [timeRange, updateHistoryTimeRangeParam];
}

export function useSelectedMetricsParam(metrics: Metric[]): [Metric[], (selectedMetrics: Metric[], replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedMetricsParam = searchParams.get('selectedMetrics') || '';
  const selectedMetricsIds = selectedMetricsParam.split(',');

  const selectedMetrics = metrics.filter((metric) => selectedMetricsIds.indexOf(metric.metricId) > -1);


  function updateSelectedMetricsParam(selectedMetrics: Metric[], replace = false) {
    const params: ParamKeyValuePair[] = [];
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'selectedMetrics') {
        params.push([k, v]);
      }
    }

    const selectedMetricsIds = selectedMetrics.map((metric) => metric.metricId).join(',');
    params.push(['selectedMetrics', selectedMetricsIds]);

    setSearchParams(params, {
      replace,
    });
  }

  return [selectedMetrics, updateSelectedMetricsParam];
}
