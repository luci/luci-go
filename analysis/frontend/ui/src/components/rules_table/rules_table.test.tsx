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

import { fireEvent, screen } from '@testing-library/react';

import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import {
  mockFetchRules,
  createDefaultMockListRulesResponse,
} from '@/testing_tools/mocks/rules_mock';

import { mockFetchProjectConfig } from '@/testing_tools/mocks/projects_mock';
import RulesTable from './rules_table';

describe('Test RulesTable component', () => {
  beforeEach(() => {
    mockFetchAuthState();
    mockFetchProjectConfig();
  });
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a project, should display the active rules', async () => {
    mockFetchRules(createDefaultMockListRulesResponse());

    renderWithRouterAndClient(
        <RulesTable
          project='chromium'/>,
        '/p/chromium/rules',
        '/p/:project/rules',
    );
    await screen.findByText('Rule Definition');

    expect(screen.getByText('crbug.com/90001')).toBeInTheDocument();
    expect(screen.getByText('test LIKE "rule1%"')).toBeInTheDocument();
    expect(screen.getByText('exonerations')).toBeInTheDocument();

    expect(screen.getByText('crbug.com/90002')).toBeInTheDocument();
    expect(screen.getByText('reason LIKE "rule2%"')).toBeInTheDocument();
    expect(screen.getByText('cls-rejected')).toBeInTheDocument();
  });

  it('if rule definitions are unavailable, filler text is displayed', async () => {
    const response = createDefaultMockListRulesResponse();
    response.rules?.forEach((r) => {
      r.ruleDefinition = '';
    });

    mockFetchRules(response);

    renderWithRouterAndClient(
        <RulesTable
          project='chromium'/>,
        '/p/chromium/rules',
        '/p/:project/rules',
    );
    await screen.findByText('Rule Definition');

    expect(screen.queryAllByText('Click to see example failures.')).toHaveLength(2);
  });

  it('problem filter filters rules', async () => {
    const response = createDefaultMockListRulesResponse();

    mockFetchRules(response);

    renderWithRouterAndClient(
        <RulesTable
          project='chromium'/>,
        '/p/chromium/rules',
        '/p/:project/rules',
    );
    await screen.findByText('Rule Definition');

    await fireEvent.change(screen.getByTestId('problem_filter_input'), { target: { value: 'exonerations' } });

    // Rule 1 visible.
    expect(screen.getByText('crbug.com/90001')).toBeInTheDocument();
    // Rule 2 not visible.
    expect(screen.queryByText('crbug.com/90002')).toBeNull();
  });
});
