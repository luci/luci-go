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

import dayjs from 'dayjs';
import fetchMock from 'fetch-mock-jest';

import { Rule } from '@/legacy_services/rules';

export const createDefaultMockRule = (): Rule => {
  return {
    name: 'projects/chromium/rules/ce83f8395178a0f2edad59fc1a167818',
    project: 'chromium',
    ruleId: 'ce83f8395178a0f2edad59fc1a167818',
    ruleDefinition: 'test = "blink_lint_expectations"',
    bug: {
      system: 'monorail',
      id: 'chromium/920702',
      linkText: 'crbug.com/920702',
      url: 'https://monorail-staging.appspot.com/p/chromium/issues/detail?id=920702',
    },
    isActive: true,
    isManagingBug: true,
    isManagingBugPriority: true,
    sourceCluster: {
      algorithm: 'testname-v3',
      id: '78ff0812026b30570ca730b1541125ea',
    },
    bugManagementState: {
      policyState: [{
        policyId: 'exonerations',
        isActive: true,
        lastActivationTime: '2022-02-01T02:34:56.123456Z',
      }, {
        policyId: 'cls-rejected',
        isActive: false,
        lastActivationTime: '2022-02-01T01:34:56.123456Z',
        lastDeactivationTime: '2022-02-01T02:04:56.123456Z',
      }],
    },
    createTime: dayjs().toISOString(),
    createUser: 'system',
    lastAuditableUpdateTime: dayjs().toISOString(),
    lastAuditableUpdateUser: 'user@example.com',
    lastUpdateTime: dayjs().toISOString(),
    predicateLastUpdateTime: '2022-01-31T03:36:14.896430Z',
    etag: 'W/"2022-01-31T03:36:14.89643Z"',
  };
};

export const mockFetchRule = (rule: Rule) => {
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Rules/Get', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(rule),
  });
};
