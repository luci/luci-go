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

import { render, screen, fireEvent } from '@testing-library/react';
import { createMockGroupIndividual, mockFetchGetGroup, mockErrorFetchingGetGroup } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import List from '@mui/material/List';

import { GroupsForm } from './groups_form';
import { mockResponseUpdateGroup, createMockUpdatedGroup, mockErrorUpdateGroup } from '../testing_tools/mocks/update_group_mock';
import { mockErrorDeleteGroup } from '../testing_tools/mocks/delete_group_mock';
import { act } from 'react';

describe('<GroupsForm />', () => {
  test('if group name, desciption, owners, members, subgroups are displayed', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    render(
        <FakeContextProvider
          mountedPath="/ui/labs/auth/groups/*"
          routerOptions={{
            initialEntries: ['/ui/labs/auth/groups/123'],
          }}
        >
        <List>
          <GroupsForm name='123' onDelete={() => {}}/>
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('groups-form');

    expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.description)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.owners)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[1])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-error');

    expect(screen.getByText('Failed to load groups form')).toBeInTheDocument();
    expect(screen.queryByTestId('groups-form')).toBeNull();
  });

  test('if external group shows only members', async () => {
    const mockGroup = createMockGroupIndividual('external/123', false, true);
    mockFetchGetGroup(mockGroup);

    render(
        <FakeContextProvider
          mountedPath="/ui/labs/auth/groups/*"
          routerOptions={{
            initialEntries: ['/ui/labs/auth/groups/external/123'],
          }}
        >
        <List>
          <GroupsForm name='external/123' onDelete={() => {}}/>
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    expect(screen.getByText(mockGroup.name)).toBeInTheDocument();
    expect(screen.queryByText(mockGroup.description)).toBeNull();
    expect(screen.queryByText(mockGroup.owners)).toBeNull();
    expect(screen.getByText(mockGroup.members[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.members[1])).toBeInTheDocument();
    expect(screen.queryByText(mockGroup.nested[0])).toBeNull();
    expect(screen.queryByText(mockGroup.nested[1])).toBeNull();
  });

  test('if group is updated with success message', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    const mockUpdatedGroup = createMockUpdatedGroup('123');
    mockResponseUpdateGroup(mockUpdatedGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupsForm name='123' onDelete={() => {}}/>
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Change something so updated button is not disabled.
    fireEvent.mouseEnter(screen.getByTestId('description-table'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());
    const descriptionTextfield = screen.getByTestId('description-textfield').querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, { target: { value: 'new description' } });
    });
    expect(descriptionTextfield!.value).toBe('new description');
    act(() => editButton.click());

    await screen.findByRole('alert');
    expect(screen.getByText('Group updated')).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error updating group', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    mockErrorUpdateGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Change something so updated button is not disabled.
    fireEvent.mouseEnter(screen.getByTestId('description-table'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());
    const descriptionTextfield = screen.getByTestId('description-textfield').querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, { target: { value: 'new description' } });
    });
    expect(descriptionTextfield!.value).toBe('new description');
    act(() => editButton.click());

    await screen.findByRole('alert');
    expect(screen.getByText('Error editing group')).toBeInTheDocument();
  });

  test('delete button opens confirm dialog', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    const mockUpdatedGroup = createMockUpdatedGroup('123');
    mockResponseUpdateGroup(mockUpdatedGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupsForm name='123' onDelete={() => {}}/>
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const deleteButton = screen.getByTestId('delete-button')
    act(() => deleteButton.click());
    expect(screen.getByTestId('delete-confirm-dialog')).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error deleting group', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    mockErrorDeleteGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const deleteButton = screen.getByTestId('delete-button')
    act(() => deleteButton.click());
    expect(screen.getByTestId('delete-confirm-dialog')).toBeInTheDocument();
    const deleteConfirmButton = screen.getByTestId('delete-confirm-button');
    act(() => deleteConfirmButton.click());
    await screen.findByRole('alert');
    expect(screen.getByText('Error deleting group')).toBeInTheDocument();
  });

  test('if edit and delete buttons are hidden if caller does not have edit permissions', async () => {
    const mockGroup = createMockGroupIndividual('123', false, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const deleteButton = screen.queryByTestId('delete-button')
    expect(deleteButton).toBeNull();
    const submitButton = screen.queryByTestId('submit-button')
    expect(submitButton).toBeNull();
    expect(screen.getByText('You do not have sufficient permissions to modify this group.')).toBeInTheDocument();
  });

  test('members redacted if caller cannot view external group', async () => {
    const mockGroup = createMockGroupIndividual('google/testGoogleGroup', true, false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='google/testGoogleGroup' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });

  test('members redacted if caller cannot view non-external group', async () => {
    const mockGroup = createMockGroupIndividual('test-group', true, false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='test-group' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });

  test('members redacted if caller cannot view or modify non-external group', async () => {
    const mockGroup = createMockGroupIndividual('test-group', false, false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='test-group' onDelete={() => {}}/>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });
});
