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
import {
  createDefaultMockRule,
  mockFetchRule,
} from '@/clusters/testing_tools/mocks/rule_mock';
import { DeepMutable } from '@/clusters/types/types';
import { Rule } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';

import BugInfo from './bug_info';

describe('Test BugInfo component', () => {
  let mockRule: DeepMutable<Rule>;

  beforeEach(() => {
    mockFetchAuthState();
    mockRule = createDefaultMockRule();
    mockFetchRule(createDefaultMockRule());
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a rule with buganizer bug, should display bug only', async () => {
    mockRule.bug = {
      system: 'buganizer',
      id: '541231',
      linkText: 'b/541231',
      url: 'https://issuetracker.google.com/issues/541231',
    };

    renderWithRouterAndClient(<BugInfo rule={mockRule} />);

    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    await waitFor(() =>
      expect(screen.getByText(mockRule.bug!.linkText)).toBeInTheDocument(),
    );
  });

  it('when clicking edit, should open dialog, even if bug does not load', async () => {
    renderWithRouterAndClient(
      <BugInfo rule={mockRule} />,
      '/ui/clusters/labs/p/chromium/rules/123456',
      '/ui/clusters/labs/p/:project/rules/:id',
    );

    await screen.findByText('Associated Bug');

    // Open the edit dialog to change the associated bug.
    fireEvent.click(screen.getByLabelText('edit'));
    expect(
      await screen.findByText('Change associated bug'),
    ).toBeInTheDocument();
  });
});
