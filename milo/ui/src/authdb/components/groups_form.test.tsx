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
import userEvent from '@testing-library/user-event'
import { createMockGroupIndividual, mockFetchGetGroup, mockErrorFetchingGetGroup } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import List from '@mui/material/List';

import { GroupsForm } from './groups_form';
import { mockResponseUpdateGroup, createMockUpdatedGroup, mockErrorUpdateGroup } from '../testing_tools/mocks/update_group_mock';
import { mockErrorDeleteGroup } from '../testing_tools/mocks/delete_group_mock';
import { act } from 'react';

describe('<GroupsForm />', () => {
  test('if group name, desciption, owners, members, subgroups are displayed', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
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
    expect(screen.getByText(mockGroup.nested[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-error');

    expect(screen.getByText('Failed to load groups form')).toBeInTheDocument();
    expect(screen.queryByTestId('groups-form')).toBeNull();
  });

  test('if external group shows only members', async () => {
    const mockGroup = createMockGroupIndividual('external/123', false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupsForm name='external/123' />
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
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    const mockUpdatedGroup = createMockUpdatedGroup('123');
    mockResponseUpdateGroup(mockUpdatedGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupsForm name='123' />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const submitButton = screen.getByTestId('submit-button')
    act(() => submitButton.click());
    await screen.findByRole('alert');
    expect(screen.getByText('Group updated')).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error updating group', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    mockErrorUpdateGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const submitButton = screen.getByTestId('submit-button')
    act(() => submitButton.click());
    await screen.findByRole('alert');
    expect(screen.getByText('Error editing group')).toBeInTheDocument();
  });

  test('delete button opens confirm dialog', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    const mockUpdatedGroup = createMockUpdatedGroup('123');
    mockResponseUpdateGroup(mockUpdatedGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupsForm name='123' />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const deleteButton = screen.getByTestId('delete-button')
    act(() => deleteButton.click());
    expect(screen.getByTestId('delete-confirm-dialog')).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error deleting group', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    mockErrorDeleteGroup();

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
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
    const mockGroup = createMockGroupIndividual('123', false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    const deleteButton = screen.queryByTestId('delete-button')
    expect(deleteButton).toBeNull();
    const submitButton = screen.queryByTestId('submit-button')
    expect(submitButton).toBeNull();
    expect(screen.getByText('You do not have sufficient permissions to modify this group.')).toBeInTheDocument();
  });

  test('reset button resets form values', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Put the Description in edit mode.
    fireEvent.mouseEnter(screen.getByTestId('description-table'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());

    // Change the group's description.
    const descriptionTextfield = screen.getByTestId('description-textfield').querySelector('input');
    act(() => {

      fireEvent.change(descriptionTextfield!, { target: { value: 'new description' } });
    });
    expect(descriptionTextfield!.value).toBe('new description');

    // Turn off edit mode for the Description.
    act(() => editButton.click());

    // The reset button should be displayed now that there are unsaved changes.
    expect(screen.getByTestId('reset-button')).toBeInTheDocument();

    // Resetting the form should restore the original description.
    act(() => screen.getByTestId('reset-button').click());
    expect(screen.getByText('testDescription')).toBeInTheDocument();
  });

  test('message shown for edited state for description', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Put the Description in edit mode.
    fireEvent.mouseEnter(screen.getByTestId('description-table'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());

    // Change the group's description.
    const descriptionTextfield = screen.getByTestId('description-textfield').querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, { target: { value: 'new description' } });
    });
    expect(descriptionTextfield!.value).toBe('new description');

    // Confirm changed Description and check message is shown.
    act(() => editButton.click());
    expect(screen.getByText('You have unsaved changes!')).toBeInTheDocument();
  });

  test('message shown for edited state for owners', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Put the owners table in edit mode.
    fireEvent.mouseEnter(screen.getByTestId('owners-table'));
    await screen.findByTestId('edit-owners-icon');
    const editButton = screen.getByTestId('edit-owners-icon');
    act(() => editButton.click());

    // Change the group's owners.
    const ownersTextfield = screen.getByTestId('owners-textfield').querySelector('input');
    act(() => {
      fireEvent.change(ownersTextfield!, { target: { value: 'new owners' } });
    });
    expect(ownersTextfield!.value).toBe('new owners');

    // Confirm changed owners and check message is shown.
    act(() => editButton.click());
    expect(screen.getByText('You have unsaved changes!')).toBeInTheDocument();
  });

  test('message shown for edited state in groups form list item', async () => {
    const mockGroup = createMockGroupIndividual('123', true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupsForm name='123' />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form');

    // Click add button for first list (members).
    const addButton = screen.queryAllByTestId('add-button')[0];
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember@email.com');
    expect(textfield!.value).toBe('newMember@email.com');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check new member shown in list & message is shown.
    expect(screen.getByText('newMember@email.com')).toBeInTheDocument();
    expect(screen.getByText('You have unsaved changes!')).toBeInTheDocument();
  });
});
