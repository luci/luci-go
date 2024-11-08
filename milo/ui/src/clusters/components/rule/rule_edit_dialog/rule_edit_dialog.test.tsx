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
/* eslint-disable @typescript-eslint/no-non-null-assertion */

import '@testing-library/jest-dom';
import 'node-fetch';

import { fireEvent, screen, waitFor } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { identityFunction } from '@/clusters/testing_tools/functions';
import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import {
  createDefaultMockRule,
  mockUpdateRule,
} from '@/clusters/testing_tools/mocks/rule_mock';
import { Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';

import RuleEditDialog from './rule_edit_dialog';

describe('Test RuleEditDialog component', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it("when modifying the rule's text, then should update the rule", async () => {
    const mockRule = createDefaultMockRule();
    mockFetchAuthState();

    renderWithRouterAndClient(
      <RuleEditDialog open rule={mockRule} setOpen={identityFunction} />,
    );

    await screen.findByTestId('rule-input');

    fireEvent.change(screen.getByTestId('rule-input'), {
      target: { value: 'new rule definition' },
    });

    const updatedRule: Rule = {
      ...mockRule,
      ruleDefinition: 'new rule definition',
    };
    mockUpdateRule(updatedRule);

    fireEvent.click(screen.getByText('Save'));
    await waitFor(
      () =>
        fetchMock.lastCall() !== undefined &&
        fetchMock.lastCall()![0] ===
          'http://localhost/prpc/luci.analysis.v1.Rules/Update',
    );

    expect(fetchMock.lastCall()![1]!.body).toEqual(
      '{"rule":{"name":"projects/chromium/rules/ce83f8395178a0f2edad59fc1a167818",' +
        '"ruleDefinition":"new rule definition"},' +
        '"updateMask":"ruleDefinition","etag":"W/\\"2022-01-31T03:36:14.89643Z\\""' +
        '}',
    );
  });

  it('when canceling the changes, then should revert', async () => {
    const mockRule = createDefaultMockRule();

    renderWithRouterAndClient(
      <RuleEditDialog open rule={mockRule} setOpen={identityFunction} />,
    );
    await screen.findByTestId('rule-input');

    fireEvent.change(screen.getByTestId('rule-input'), {
      target: { value: 'new rule definition' },
    });

    fireEvent.click(screen.getByText('Cancel'));

    expect(screen.getByTestId('rule-input')).toHaveValue(
      'test = "blink_lint_expectations"',
    );
  });
});
