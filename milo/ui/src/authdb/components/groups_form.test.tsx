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

import { createMockGroupIndividual, mockFetchGetGroup, mockErrorFetchingGetGroup } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import List from '@mui/material/List';

import { GroupsForm } from './groups_form';

describe('<GroupsListItem />', () => {
  test('if group name, desciption, owners, members, subgroups are displayed', async () => {
    const mockGroup = createMockGroupIndividual('123');
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
            <GroupsForm name='123' />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form');

    expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.description)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.owners)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[1])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetGroup();

    render(
        <FakeContextProvider>
            <GroupsForm name='123'/>
        </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-error');

    expect(screen.getByText('Failed to load groups form')).toBeInTheDocument();
    expect(screen.queryByTestId('groups-form')).toBeNull();
  });
});
