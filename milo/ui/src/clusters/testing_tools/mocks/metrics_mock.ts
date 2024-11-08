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

import {
  ListProjectMetricsRequest,
  ListProjectMetricsResponse,
  ProjectMetric,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

export const getMockMetricsList = (project: string): ProjectMetric[] => {
  const humanClsFailedPresubmitMetric: ProjectMetric = {
    name: 'projects/' + project + '/metrics/human-cls-failed-presubmit',
    metricId: 'human-cls-failed-presubmit',
    humanReadableName: 'User Cls Failed Presubmit',
    description: 'User Cls Failed Presubmit Description',
    isDefault: true,
    sortPriority: 30,
  };
  const criticalFailuresExonerated: ProjectMetric = {
    name: 'projects/' + project + '/metrics/critical-failures-exonerated',
    metricId: 'critical-failures-exonerated',
    humanReadableName: 'Presubmit-blocking Failures Exonerated',
    description: 'Critical Failures Exonerated Description',
    isDefault: true,
    sortPriority: 40,
  };
  const testRunsFailed: ProjectMetric = {
    name: 'projects/' + project + '/metrics/test-runs-failed',
    metricId: 'test-runs-failed',
    humanReadableName: 'Test Runs Failed',
    description: 'Test Runs Failed Description',
    isDefault: false,
    sortPriority: 20,
  };
  const failures: ProjectMetric = {
    name: 'projects/' + project + '/metrics/failures',
    metricId: 'failures',
    humanReadableName: 'Total Failures',
    description: 'Test Results Failed Description',
    isDefault: true,
    sortPriority: 10,
  };
  return [
    humanClsFailedPresubmitMetric,
    criticalFailuresExonerated,
    testRunsFailed,
    failures,
  ];
};

export const mockFetchMetrics = (
  project?: string,
  metrics?: ProjectMetric[],
) => {
  if (project === undefined) {
    project = 'testproject';
  }
  if (metrics === undefined) {
    metrics = getMockMetricsList(project);
  }
  const request: ListProjectMetricsRequest = {
    parent: 'projects/' + project,
  };
  const response: ListProjectMetricsResponse = {
    metrics: metrics,
  };

  fetchMock.post(
    {
      url: 'https://staging.analysis.api.luci.app/prpc/luci.analysis.v1.Metrics/ListForProject',
      body: ListProjectMetricsRequest.toJSON(request) as object,
    },
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'\n" + JSON.stringify(ListProjectMetricsResponse.toJSON(response)),
    },
  );
};
