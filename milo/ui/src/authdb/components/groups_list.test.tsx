// Copyright 2024 The LUCI Authors.
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

import { render, screen } from '@testing-library/react';

import { createMockGroup } from '@/authdb/testing_tools/mocks/group_mock';
import { mockFetchGroups, mockErrorFetchingGroups } from '@/authdb/testing_tools/mocks/groups_list_mock';

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { GroupsList } from './groups_list';
import fetchMock from 'fetch-mock-jest';

describe('<GroupsList />', () => {
  afterEach(() => {
    fetchMock.mockClear();
    fetchMock.reset();
  });
  test('if groups list are displayed', async () => {
    const mockGroups = [
        createMockGroup('123'),
        createMockGroup('124'),
        createMockGroup('125'),
    ];
    mockFetchGroups(mockGroups);

    render(
      <FakeContextProvider>
            <GroupsList />
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-list');

    mockGroups.forEach((mockGroup) => {
        expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
    });
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGroups();

    render(
        <FakeContextProvider>
            <GroupsList />
        </FakeContextProvider>,
    );
    await screen.findByTestId('groups-list-error');

    expect(screen.getByText('Failed to load groups list')).toBeInTheDocument();
    expect(screen.queryByTestId('groups-list')).toBeNull();
  });
});