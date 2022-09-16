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

import { Rule } from '@/services/rules';

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
    sourceCluster: {
      algorithm: 'testname-v3',
      id: '78ff0812026b30570ca730b1541125ea',
    },
    createTime: dayjs().toISOString(),
    createUser: 'system',
    lastUpdateTime: dayjs().toISOString(),
    lastUpdateUser: 'user@example.com',
    predicateLastUpdateTime: '2022-01-31T03:36:14.896430Z',
    etag: 'W/"2022-01-31T03:36:14.89643Z"',
  };
};

export const mockFetchRule = () => {
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Rules/Get', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'' + JSON.stringify(createDefaultMockRule()),
  });
};
