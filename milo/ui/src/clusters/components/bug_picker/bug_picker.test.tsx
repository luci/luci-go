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

import { screen } from '@testing-library/react';
import fetchMock from 'fetch-mock-jest';

import { identityFunction } from '@/clusters/testing_tools/functions';
import { renderWithRouterAndClient } from '@/clusters/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/clusters/testing_tools/mocks/authstate_mock';
import { mockFetchProjectConfig } from '@/clusters/testing_tools/mocks/projects_mock';

import BugPicker from './bug_picker';

describe('Test BugPicker component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a buganizer bug, should select the bug system correctly', async () => {
    mockFetchProjectConfig();

    renderWithRouterAndClient(
      <BugPicker bugId="123456" handleBugIdChanged={identityFunction} />,
      '/chromium',
      '/:project',
    );
    await screen.findByTestId('bug-number');
    expect(screen.getByTestId('bug-number')).toHaveValue('123456');
  });
});
