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

import { screen } from '@testing-library/react';

import { renderWithRouterAndClient } from '../../testing_tools/libs/mock_router';
import { mockFetchAuthState } from '../../testing_tools/mocks/authstate_mock';
import { mockFetchRules } from '../../testing_tools/mocks/rules_mock';
import RulesTable from './rules_table';

describe('Test RulesTable component', () => {
  it('given a project, should display the active rules', async () => {
    mockFetchAuthState();
    mockFetchRules();

    renderWithRouterAndClient(
        <RulesTable
          project='chromium'/>,
        '/p/chromium/rules',
        '/p/:project/rules',
    );
    await screen.findByText('Rule Definition');

    expect(screen.getByText('crbug.com/90001')).toBeInTheDocument();
    expect(screen.getByText('crbug.com/90002')).toBeInTheDocument();
    expect(screen.getByText('test LIKE "rule1%"')).toBeInTheDocument();
    expect(screen.getByText('reason LIKE "rule2%"')).toBeInTheDocument();
  });
});
