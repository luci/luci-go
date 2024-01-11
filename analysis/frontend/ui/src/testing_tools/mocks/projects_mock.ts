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

import { BugManagementPolicy, ProjectConfig } from '@/legacy_services/project';

export const createMockExonerationsPolicy = (): BugManagementPolicy => {
  return {
    id: 'exonerations',
    owners: ['exoneration-owner1@google.com', 'exoneration-owner2@google.com'],
    humanReadableName: 'test variant(s) are being exonerated (ignored) in presubmit',
    priority: 'P2',
    metrics: [{
      metricId: 'critical-failures-exonerated',
      activationThreshold: {
        threeDay: '75',
      },
      deactivationThreshold: {
        sevenDay: '1',
      },
    }],
    explanation: {
      problemHtml: 'Test variant(s) in this cluster are too flaky to gate changes in CQ. As a result, functionality is not protected from breakage.',
      actionHtml: '<ul><li>Review the exonerations tab to see which test variants are being exonerated.</li><li>View recent failures to help you debug the problem.</li></ul>',
    },
  };
};

export const createMockProjectConfig = (): ProjectConfig => {
  return {
    name: 'projects/chromium/config',
    bugManagement: {
      policies: [createMockExonerationsPolicy(), {
        id: 'cls-rejected',
        owners: ['cls-rejected-owner1@google.com', 'cls-rejected-owner2@google.com'],
        humanReadableName: 'many CL(s) are being rejected',
        priority: 'P0',
        metrics: [{
          metricId: 'human-cls-failed-presubmit',
          activationThreshold: {
            threeDay: '10',
          },
          deactivationThreshold: {
            sevenDay: '1',
          },
        }],
        explanation: {
          problemHtml: 'Many changelists are unable to submit because of failures on this test.',
          actionHtml: '<ul><li>Consider disabling the test(s) while you investigate the root cause, to immediately unblock developers.</li><li>View recent failures to help you debug the problem.</li></ul>',
        },
      }],
      monorail: {
        project: 'chromium',
        displayPrefix: 'crbug.com',
      },
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
