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

import fetchMock from 'fetch-mock-jest';

import {
  fireEvent,
  screen,
} from '@testing-library/react';

import { BugManagement, ProjectConfig } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';
import { identityFunction } from '@/testing_tools/functions';
import { renderWithRouterAndClient } from '@/testing_tools/libs/mock_router';
import { mockFetchAuthState } from '@/testing_tools/mocks/authstate_mock';
import { mockFetchProjectConfig } from '@/testing_tools/mocks/projects_mock';

import BugPicker from './bug_picker';

describe('Test BugPicker component', () => {
  beforeEach(() => {
    mockFetchAuthState();
  });

  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });

  it('given a bug and a project, should display select and a text box for writing the bug id', async () => {
    mockFetchProjectConfig();

    renderWithRouterAndClient(
        <BugPicker
          bugId="chromium/123456"
          bugSystem="monorail"
          handleBugSystemChanged={identityFunction}
          handleBugIdChanged={identityFunction}/>, '/p/chromium', '/p/:project');
    await screen.findByText('Bug tracker');
    expect(screen.getByTestId('bug-system')).toHaveValue('monorail');
    expect(screen.getByTestId('bug-number')).toHaveValue('123456');
  });

  it('given a buganizer bug, should select the bug system correctly', async () => {
    mockFetchProjectConfig();

    renderWithRouterAndClient(
        <BugPicker
          bugId="123456"
          bugSystem="buganizer"
          handleBugSystemChanged={identityFunction}
          handleBugIdChanged={identityFunction}/>, '/p/chromium', '/p/:project');
    await screen.findByText('Bug tracker');
    expect(screen.getByTestId('bug-system')).toHaveValue('buganizer');
    expect(screen.getByTestId('bug-number')).toHaveValue('123456');
  });

  it('handles project config missing monorail details', async () => {
    const bareProjectConfig: ProjectConfig = {
      name: 'projects/chromium/config',
      bugManagement: BugManagement.create(),
    };
    mockFetchProjectConfig(bareProjectConfig);

    renderWithRouterAndClient(
        <BugPicker
          bugId="123456"
          bugSystem="buganizer"
          handleBugSystemChanged={identityFunction}
          handleBugIdChanged={identityFunction} />, '/p/chromium', '/p/:project');
    await screen.findByText('Bug tracker');
    expect(screen.getByTestId('bug-system')).toHaveValue('buganizer');
    expect(screen.getByTestId('bug-number')).toHaveValue('123456');

    await fireEvent.mouseDown(screen.getByRole('button'));
    const monorailOption = screen.getByText('monorail');
    expect(monorailOption).toBeInTheDocument();
    expect(monorailOption).toHaveAttribute('aria-disabled');
  });
});
