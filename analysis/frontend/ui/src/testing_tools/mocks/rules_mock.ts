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

import { ListRulesResponse } from '@/legacy_services/rules';

import { createDefaultMockRule } from './rule_mock';

export const createDefaultMockListRulesResponse = (): ListRulesResponse => {
  const rule1 = createDefaultMockRule();
  rule1.name = 'projects/chromium/rules/ce83f8395178a0f2edad59fc1a160001';
  rule1.ruleId = 'ce83f8395178a0f2edad59fc1a160001';
  rule1.bug = {
    system: 'monorail',
    id: 'chromium/90001',
    linkText: 'crbug.com/90001',
    url: 'https://monorail-staging.appspot.com/p/chromium/issues/detail?id=90001',
  };
  rule1.ruleDefinition = 'test LIKE "rule1%"';
  rule1.bugManagementState = {
    policyState: [{
      policyId: 'exonerations',
      isActive: true,
      lastActivationTime: '2022-02-01T01:34:56.123456Z',
    }],
  };

  const rule2 = createDefaultMockRule();
  rule2.name = 'projects/chromium/rules/ce83f8395178a0f2edad59fc1a160002';
  rule2.ruleId = 'ce83f8395178a0f2edad59fc1a160002';
  rule2.bug = {
    system: 'monorail',
    id: 'chromium/90002',
    linkText: 'crbug.com/90002',
    url: 'https://monorail-staging.appspot.com/p/chromium/issues/detail?id=90002',
  };
  rule2.ruleDefinition = 'reason LIKE "rule2%"';
  rule2.bugManagementState = {
    policyState: [{
      policyId: 'cls-rejected',
      isActive: false,
      lastActivationTime: '2022-02-01T01:34:56.123456Z',
      lastDeactivationTime: '2022-02-01T02:04:56.123456Z',
    }],
  };
  return {
    rules: [rule1, rule2],
  };
};

export const mockFetchRules = (response: ListRulesResponse) => {
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Rules/List', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'' + JSON.stringify(response),
  });
};
