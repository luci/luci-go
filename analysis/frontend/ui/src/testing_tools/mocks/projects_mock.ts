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

import { ProjectConfig } from '@/services/project';

export const createMockProjectConfig = (): ProjectConfig => {
  return {
    name: 'projects/chromium/config',
    monorail: {
      project: 'chromium',
      displayPrefix: 'crbug.com',
    },
  };
};

export const createMockProjectConfigWithThresholds = (): ProjectConfig => {
  return {
    name: 'projects/chromium/config',
    monorail: {
      project: 'chromium',
      displayPrefix: 'crbug.com',
      priorities: [
        {
          priority: 'P0',
          thresholds: [
            {
              metricId: 'human-cls-failed-presubmit',
              threshold: {
                oneDay: '20',
              },
            },
          ],
        },
        {
          priority: 'P1',
          thresholds: [
            {
              metricId: 'human-cls-failed-presubmit',
              threshold: {
                oneDay: '10',
              },
            },
          ],
        },
      ],
    },
  };
};

export const mockFetchProjectConfig = (projectConfig?: ProjectConfig) => {
  if (projectConfig === undefined) {
    projectConfig = createMockProjectConfig();
  }
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Projects/GetConfig', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'' + JSON.stringify(projectConfig),
  });
};
