// Copyright 2026 The LUCI Authors.
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

import { screen } from '@testing-library/react';
import * as React from 'react';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  createMockProjectConfig,
  mockFetchProjectConfig,
} from '@/clusters/testing_tools/mocks/projects_mock';
import {
  createDefaultMockRule,
  mockFetchRule,
} from '@/clusters/testing_tools/mocks/rule_mock';
import { resetMockFetch } from '@/testing_tools/jest_utils';

import { RulePage } from './rule';

jest.mock('@/clusters/components/rule/rule_top_panel/rule_top_panel', () => ({
  __esModule: true,
  default: () =>
    React.createElement('div', { 'data-testid': 'mock-rule-top-panel' }),
}));

jest.mock(
  '@/clusters/components/cluster/cluster_analysis_section/cluster_analysis_section',
  () => ({
    __esModule: true,
    default: () =>
      React.createElement('div', {
        'data-testid': 'mock-cluster-analysis-section',
      }),
  }),
);

describe('Test RulePage', () => {
  const project = 'chrome';
  const id = '123456';

  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    resetMockFetch();
  });

  it('given active rule, should render top panel, problems section and analysis section', async () => {
    const rule = createDefaultMockRule();
    rule.isActive = true;
    rule.bugManagementState!.policyState = [
      {
        policyId: 'exonerations',
        isActive: true,
        lastActivationTime: '2022-02-01T02:34:56.123456Z',
        lastDeactivationTime: undefined,
      },
    ];
    mockFetchRule(rule);
    mockFetchProjectConfig(createMockProjectConfig());

    renderWithRouterAndClient(
      <RulePage />,
      `/p/${project}/rules/${id}`,
      '/p/:project/rules/:id',
    );

    // Wait for rule to load (RulePage shows LinearProgress while pending)
    await screen.findByTestId('mock-rule-top-panel');
    expect(
      await screen.findByText(
        'test variant(s) are being exonerated (ignored) in presubmit',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId('mock-cluster-analysis-section'),
    ).toBeInTheDocument();
  });

  it('given inactive rule, should render top panel and archived message, but no problems or analysis section', async () => {
    const rule = createDefaultMockRule();
    rule.isActive = false;
    mockFetchRule(rule);

    renderWithRouterAndClient(
      <RulePage />,
      `/p/${project}/rules/${id}`,
      '/p/:project/rules/:id',
    );

    await screen.findByTestId('mock-rule-top-panel');
    expect(screen.queryByText('Problems')).toBeNull();
    expect(screen.queryByTestId('mock-cluster-analysis-section')).toBeNull();
    expect(screen.getByText('This Rule is Archived')).toBeInTheDocument();
  });
});
