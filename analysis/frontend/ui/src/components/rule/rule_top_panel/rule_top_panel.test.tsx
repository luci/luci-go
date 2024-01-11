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

import '@testing-library/jest-dom';
import 'node-fetch';

import fetchMock from 'fetch-mock-jest';

import { screen } from '@testing-library/react';

import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { createMockBug } from '@/testing_tools/mocks/bug_mock';
import { mockReclusteringProgress } from '@/testing_tools/mocks/cluster_mock';
import { createMockDoneProgress } from '@/testing_tools/mocks/progress_mock';
import { mockFetchProjectConfig } from '@/testing_tools/mocks/projects_mock';
import { createDefaultMockRule } from '@/testing_tools/mocks/rule_mock';

import RuleTopPanel from './rule_top_panel';

describe('Test RuleTopPanel component', () => {
  it('given a rule, should display rule and bug details', async () => {
    mockFetchProjectConfig();
    mockFetchAuthState();
    const mockRule = createDefaultMockRule();
    mockReclusteringProgress(createMockDoneProgress());
    fetchMock.post('https://api-dot-crbug.com/prpc/monorail.v3.Issues/GetIssue', {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ')]}\'\n' + JSON.stringify(createMockBug()),
    });
    fetchMock.post('http://localhost/prpc/luci.analysis.v1.Rules/Get', {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ')]}\'\n'+JSON.stringify(mockRule),
    });

    renderWithRouterAndClient(
        <RuleTopPanel
          project="chromium"
          ruleId='12345'/>,
        '/p/chromium/rules/12345',
        '/p/:project/rules/:id',
    );
    await screen.findByText('Rule Details');

    expect(screen.getByText('Rule Details')).toBeInTheDocument();
    expect(screen.getByText('Associated Bug')).toBeInTheDocument();
  });
});
