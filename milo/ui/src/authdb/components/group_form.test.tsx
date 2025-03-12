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

import List from '@mui/material/List';
import { render, screen, fireEvent } from '@testing-library/react';
import { act } from 'react';

import { stripPrefix } from '@/authdb/common/helpers';
import {
  createMockGroupIndividual,
  mockFetchGetGroup,
  mockErrorFetchingGetGroup,
} from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { mockErrorDeleteGroup } from '../testing_tools/mocks/delete_group_mock';
import {
  mockResponseUpdateGroup,
  createMockUpdatedGroup,
  mockErrorUpdateGroup,
} from '../testing_tools/mocks/update_group_mock';

import { GroupForm } from './group_form';

describe('<GroupForm />', () => {
  test('if group name, desciption, owners, members, subgroups are displayed', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider
        mountedPath="/ui/auth/groups/*"
        routerOptions={{
          initialEntries: ['/ui/auth/groups/123'],
        }}
      >
        <List>
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('group-form');

    expect(screen.getByText(mockGroup.description)).toBeInTheDocument();
    expect(screen.getByText(mockGroup.owners)).toBeInTheDocument();
    const editedMembers = mockGroup.members.map((member) =>
      stripPrefix('user', member),
    );
    expect(screen.getByText(editedMembers[0])).toBeInTheDocument();
    expect(screen.getByText(editedMembers[1])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[0])).toBeInTheDocument();
    expect(screen.getByText(mockGroup.nested[1])).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error', async () => {
    mockErrorFetchingGetGroup();

    render(
      <FakeContextProvider>
        <GroupForm name="123" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form-error');

    expect(screen.getByText('Failed to load groups form')).toBeInTheDocument();
    expect(screen.queryByTestId('group-form')).toBeNull();
  });

  test('if external group shows only members', async () => {
    const mockGroup = createMockGroupIndividual('external/123', false, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider
        mountedPath="/ui/auth/groups/*"
        routerOptions={{
          initialEntries: ['/ui/auth/groups/external/123'],
        }}
      >
        <List>
          <GroupForm name="external/123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');
    const editedMembers = mockGroup.members.map((member) =>
      stripPrefix('user', member),
    );
    expect(screen.queryByText(mockGroup.description)).toBeNull();
    expect(screen.queryByText(mockGroup.owners)).toBeNull();
    expect(screen.getByText(editedMembers[0])).toBeInTheDocument();
    expect(screen.getByText(editedMembers[1])).toBeInTheDocument();
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
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    // Change something so updated button is not disabled.
    fireEvent.mouseEnter(screen.getByTestId('description-row'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());
    const descriptionTextfield = screen
      .getByTestId('description-textfield')
      .querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, {
        target: { value: 'new description' },
      });
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
        <GroupForm name="123" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    // Change something so updated button is not disabled.
    fireEvent.mouseEnter(screen.getByTestId('description-row'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());
    const descriptionTextfield = screen
      .getByTestId('description-textfield')
      .querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, {
        target: { value: 'new description' },
      });
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
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    const deleteButton = screen.getByTestId('delete-button');
    act(() => deleteButton.click());
    expect(screen.getByTestId('delete-confirm-dialog')).toBeInTheDocument();
  });

  test('if appropriate message is displayed for an error deleting group', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    mockErrorDeleteGroup();

    render(
      <FakeContextProvider>
        <GroupForm name="123" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    const deleteButton = screen.getByTestId('delete-button');
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
        <GroupForm name="123" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    const deleteButton = screen.queryByTestId('delete-button');
    expect(deleteButton).toBeNull();
    const submitButton = screen.queryByTestId('submit-button');
    expect(submitButton).toBeNull();
    expect(
      screen.getByText(
        'You do not have sufficient permissions to modify this group.',
      ),
    ).toBeInTheDocument();
  });

  test('members redacted if caller cannot view external group', async () => {
    const mockGroup = createMockGroupIndividual(
      'google/testGoogleGroup',
      true,
      false,
    );
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupForm name="google/testGoogleGroup" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });

  test('members redacted if caller cannot view non-external group', async () => {
    const mockGroup = createMockGroupIndividual('test-group', true, false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupForm name="test-group" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });

  test('members redacted if caller cannot view or modify non-external group', async () => {
    const mockGroup = createMockGroupIndividual('test-group', false, false);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <GroupForm name="test-group" refetchList={() => {}} />
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    expect(screen.getByText('2 members redacted')).toBeInTheDocument();
  });

  test('error is shown on empty description', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('group-form');

    fireEvent.mouseEnter(screen.getByTestId('description-row'));
    await screen.findByTestId('edit-description-icon');
    const editButton = screen.getByTestId('edit-description-icon');
    act(() => editButton.click());
    const descriptionTextfield = screen
      .getByTestId('description-textfield')
      .querySelector('input');
    act(() => {
      fireEvent.change(descriptionTextfield!, { target: { value: '' } });
    });
    act(() => editButton.click());
    expect(screen.getByText('Description is required.')).toBeInTheDocument();
  });

  test('empty owners is allowed', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    const mockUpdatedGroup = createMockUpdatedGroup('123');
    mockResponseUpdateGroup(mockUpdatedGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );

    await screen.findByTestId('group-form');

    fireEvent.mouseEnter(screen.getByTestId('owners-table'));
    await screen.findByTestId('edit-owners-icon');
    const editButton = screen.getByTestId('edit-owners-icon');
    act(() => editButton.click());
    const textfield = screen
      .getByTestId('owners-textfield')
      .querySelector('input');
    fireEvent.change(textfield!, { target: { value: '' } });
    act(() => editButton.click());
    await screen.findByRole('alert');
    expect(screen.getByText('Group updated')).toBeInTheDocument();
  });

  test('error is shown on invalid owners name', async () => {
    render(
      <FakeContextProvider>
        <List>
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    fireEvent.mouseEnter(screen.getByTestId('owners-table'));
    await screen.findByTestId('edit-owners-icon');
    const editButton = screen.getByTestId('edit-owners-icon');
    act(() => editButton.click());
    const textfield = screen
      .getByTestId('owners-textfield')
      .querySelector('input');
    fireEvent.change(textfield!, { target: { value: 'Invalid owners' } });
    act(() => editButton.click());
    expect(
      screen.getByText('Invalid owners name. Must be a group.'),
    ).toBeInTheDocument();
  });

  test('owners group links correctly', async () => {
    const mockGroup = createMockGroupIndividual('123', true, true);
    mockFetchGetGroup(mockGroup);

    render(
      <FakeContextProvider>
        <List>
          <GroupForm name="123" refetchList={() => {}} />
        </List>
      </FakeContextProvider>,
    );
    await screen.findByTestId('group-form');

    expect(screen.getByText(mockGroup.owners)).toBeInTheDocument();
    expect(screen.getByTestId(`${mockGroup.owners}-link`)).toHaveAttribute(
      'href',
      `/ui/auth/groups/${mockGroup.owners}`,
    );
  });
});
