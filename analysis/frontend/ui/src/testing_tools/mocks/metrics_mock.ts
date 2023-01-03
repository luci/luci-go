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

import fetchMock from 'fetch-mock-jest';

import { ListMetricsResponse, Metric } from '@/services/metrics';

export const getMockMetricsList = (): Metric[] => {
  const humanClsFailedPresubmitMetric : Metric = {
    name: 'metrics/human-cls-failed-presubmit',
    metricId: 'human-cls-failed-presubmit',
    humanReadableName: 'User Cls Failed Presubmit',
    description: 'User Cls Failed Presubmit Description',
    isDefault: true,
    sortPriority: 30,
  };
  const criticalFailuresExonerated : Metric = {
    name: 'metrics/critical-failures-exonerated',
    metricId: 'critical-failures-exonerated',
    humanReadableName: 'Presubmit-blocking Failures Exonerated',
    description: 'Critical Failures Exonerated Description',
    isDefault: true,
    sortPriority: 40,
  };
  const testRunsFailed : Metric = {
    name: 'metrics/test-runs-failed',
    metricId: 'test-runs-failed',
    humanReadableName: 'Test Runs Failed',
    description: 'Test Runs Failed Description',
    isDefault: false,
    sortPriority: 20,
  };
  const failures : Metric = {
    name: 'metrics/failures',
    metricId: 'failures',
    humanReadableName: 'Total Failures',
    description: 'Test Results Failed Description',
    isDefault: true,
    sortPriority: 10,
  };
  return [humanClsFailedPresubmitMetric, criticalFailuresExonerated, testRunsFailed, failures];
};

export const mockFetchMetrics = (metrics?: Metric[]) => {
  if (metrics === undefined) {
    metrics = getMockMetricsList();
  }
  const response: ListMetricsResponse = {
    metrics: metrics,
  };
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Metrics/List', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'' + JSON.stringify(response),
  });
};
