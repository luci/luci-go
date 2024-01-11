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

import {
  fireEvent,
  screen,
  waitFor,
} from '@testing-library/react';

import { Issue } from '@/legacy_services/monorail';
import { Rule } from '@/legacy_services/rules';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { createMockBug } from '@/testing_tools/mocks/bug_mock';
import { mockFetchProjectConfig } from '@/testing_tools/mocks/projects_mock';
import {
  createDefaultMockRule,
  mockFetchRule,
} from '@/testing_tools/mocks/rule_mock';

import BugInfo from './bug_info';

describe('Test BugInfo component', () => {
  let mockRule!: Rule;
  let mockIssue!: Issue;

  beforeEach(() => {
    mockFetchAuthState();
    mockRule = createDefaultMockRule();
    mockIssue = createMockBug();
    mockFetchRule(createDefaultMockRule());
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a rule with monorail bug, should fetch and display bug info', async () => {
    fetchMock.post('https://api-dot-crbug.com/prpc/monorail.v3.Issues/GetIssue', {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ')]}\'' + JSON.stringify(mockIssue),
    });

    renderWithRouterAndClient(
        <BugInfo
          rule={mockRule}/>,
    );

    expect(screen.getByText(mockRule.bug.linkText)).toBeInTheDocument();

    await screen.findByText('Status');
    expect(screen.getByText(mockIssue.summary)).toBeInTheDocument();
    expect(screen.getByText(mockIssue.status.status)).toBeInTheDocument();
    expect(screen.getByText('Update bug priority')).toBeInTheDocument();
  });

  it('When bug updates are off, then should not display Update bug priority toggle', async () => {
    fetchMock.post('https://api-dot-crbug.com/prpc/monorail.v3.Issues/GetIssue', {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ')]}\'' + JSON.stringify(mockIssue),
    });

    mockRule.isManagingBug = false;

    renderWithRouterAndClient(
        <BugInfo
          rule={mockRule}/>,
    );

    expect(screen.getByText(mockRule.bug.linkText)).toBeInTheDocument();

    await screen.findByText('Status');
    expect(screen.getByText(mockIssue.summary)).toBeInTheDocument();
    expect(screen.getByText(mockIssue.status.status)).toBeInTheDocument();
    expect(screen.queryByText('Update bug priority')).not.toBeInTheDocument();
  });

  it('given a rule with buganizer bug, should display bug only', async () => {
    mockRule.bug = {
      system: 'buganizer',
      id: '541231',
      linkText: 'b/541231',
      url: 'https://issuetracker.google.com/issues/541231',
    };

    renderWithRouterAndClient(
        <BugInfo rule={mockRule}/>,
    );

    await waitFor(() => expect(screen.getByText(mockRule.bug.linkText)).toBeInTheDocument());
  });


  it('when clicking edit, should open dialog, even if bug does not load', async () => {
    // Check we can still edit the bug, even if the bug fails to load.
    fetchMock.post('https://api-dot-crbug.com/prpc/monorail.v3.Issues/GetIssue', {
      status: 404,
      headers: {
        'X-Prpc-Grpc-Code': '5',
      },
      body: 'Issue(s) not found',
    });

    mockFetchProjectConfig();
    renderWithRouterAndClient(
        <BugInfo
          rule={mockRule}/>,
        '/p/chromium/rules/123456',
        '/p/:project/rules/:id',
    );

    await screen.findByText('Associated Bug');

    // Open the edit dialog to change the associated bug.
    fireEvent.click(screen.getByLabelText('edit'));
    expect(await screen.findByText('Change associated bug')).toBeInTheDocument();
  });
});
