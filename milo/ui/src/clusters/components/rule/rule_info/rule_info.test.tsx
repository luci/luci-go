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

import { fireEvent, screen, waitFor } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  createDefaultMockRule,
  mockUpdateRule,
} from '@/clusters/testing_tools/mocks/rule_mock';
import { Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

import RuleInfo from './rule_info';

describe('Test RuleInfo component', () => {
  it('given a rule, then should display rule details', async () => {
    const mockRule = createDefaultMockRule();
    renderWithRouterAndClient(<RuleInfo project="chromium" rule={mockRule} />);

    await screen.findByText('Rule Details');

    expect(screen.getByText(mockRule.ruleDefinition)).toBeInTheDocument();
    expect(
      screen.getByText(
        `${mockRule.sourceCluster!.algorithm}/${mockRule.sourceCluster!.id}`,
      ),
    ).toBeInTheDocument();
    expect(screen.getByText('Archived')).toBeInTheDocument();
    expect(screen.getByText('No')).toBeInTheDocument();
  });

  it('when clicking on archived, then should show confirmation dialog', async () => {
    const mockRule = createDefaultMockRule();

    renderWithRouterAndClient(<RuleInfo project="chromium" rule={mockRule} />);
    await screen.findByText('Rule Details');

    fireEvent.click(screen.getByText('Archive'));
    await screen.findByText('Are you sure?');

    expect(screen.getByText('Confirm')).toBeInTheDocument();
  });

  it('when confirming the archival, then should send archival request', async () => {
    mockFetchAuthState();
    const mockRule = createDefaultMockRule();
    renderWithRouterAndClient(<RuleInfo project="chromium" rule={mockRule} />);
    await screen.findByText('Rule Details');

    fireEvent.click(screen.getByText('Archive'));
    await screen.findByText('Are you sure?');

    expect(screen.getByText('Confirm')).toBeInTheDocument();

    const updatedRule: Rule = {
      ...mockRule,
      isActive: false,
    };
    mockUpdateRule(updatedRule);

    fireEvent.click(screen.getByText('Confirm'));

    await waitFor(
      () =>
        fetchMock.lastCall() !== undefined &&
        fetchMock.lastCall()![0] ===
          'http://localhost/prpc/luci.analysis.v1.Rules/Update',
    );

    expect(fetchMock.lastCall()![1]!.body).toEqual(
      '{"rule":{"name":"projects/chromium/rules/ce83f8395178a0f2edad59fc1a167818"},' +
        '"updateMask":"isActive","etag":"W/\\"2022-01-31T03:36:14.89643Z\\""' +
        '}',
    );
  });
});
