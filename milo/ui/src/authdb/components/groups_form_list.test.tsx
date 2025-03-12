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
import userEvent from '@testing-library/user-event';
import { act } from 'react';

import { stripPrefix } from '@/authdb/common/helpers';
import { createMockGroupIndividual } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { GroupsFormList } from './groups_form_list';

describe('<GroupsFormList editable/>', () => {
  const mockGroup = createMockGroupIndividual('123', true, true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormList
          name="Members"
          initialValues={mockGroup.members as string[]}
          submitValues={() => {}}
        />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-list');
  });
  test('displays items', async () => {
    const editedMembers = mockGroup.members.map((member) =>
      stripPrefix('user', member),
    );
    // Check each member is displayed.
    for (let i = 0; i < editedMembers.length; i++) {
      expect(screen.getByText(editedMembers[i])).toBeInTheDocument();
    }
    // Check no remove button exists on readonly.
    expect(screen.queryAllByTestId('remove-button')).toHaveLength(0);
  });

  test('shows removed members confirm dialog', async () => {
    const editedMembers = mockGroup.members.map((member) =>
      stripPrefix('user', member),
    );
    // Simulate mouse enter event each row.
    for (let i = 0; i < mockGroup.members.length; i++) {
      const row = screen.getByTestId(`item-row-${editedMembers[i]}`);
      fireEvent.mouseEnter(row);
      const removeCheckbox = screen
        .getByTestId(`checkbox-button-${editedMembers[i]}`)
        .querySelector('input');
      act(() => removeCheckbox!.click());
      const removeButton = screen.getByTestId('remove-button');
      act(() => removeButton.click());
      expect(screen.queryByTestId('remove-confirm-dialog')).not.toBeNull();
    }
  });

  test('shows add button', async () => {
    // Check that remove button exists for each member.
    const addButton = screen.queryByTestId('add-button');
    // Check add button appears on edit.
    expect(addButton).not.toBeNull();
  });

  test('can add members', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember@email.com');
    expect(textfield!.value).toBe('newMember@email.com');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check new member shown in list?
    expect(screen.getByText('newMember@email.com')).toBeInTheDocument();
  });

  test('shows error message on adding email with no @ symbol', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(screen.getByText('Invalid Members: newMember')).toBeInTheDocument();
  });

  test('shows error message on invalid email', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember@email/com');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(
      screen.getByText('Invalid Members: newMember@email/com'),
    ).toBeInTheDocument();
  });

  test('shows error message on duplicate item added (case-insensitive)', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    // Add member that already exists.
    await userEvent.type(textfield!, 'member1@Email.com');
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(
      screen.getByText('Invalid Members: member1@Email.com'),
    ).toBeInTheDocument();
  });

  test('hides textfield with clear button', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    // Click clear button.
    const clearButton = screen.queryByTestId('clear-button');
    expect(clearButton).not.toBeNull();
    act(() => clearButton!.click());
    // Check textfield is no longer shown.
    await screen.findByTestId('add-button');
    expect(textfield).not.toBeInTheDocument();
  });
});

describe('<GroupsFormList editable globs/>', () => {
  const mockGroup = createMockGroupIndividual('123', true, true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormList
          name="Globs"
          initialValues={mockGroup.globs as string[]}
          submitValues={() => {}}
        />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-list');
  });
  test('shows error message on adding glob without a * symbol', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, '.glob');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(screen.getByText('Invalid Globs: user:.glob')).toBeInTheDocument();
  });
  test('shows error message on duplicate item added, case-sensitive', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    await userEvent.type(textfield!, '*@email.com');
    // Add member that already exists.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(
      screen.getByText('Invalid Globs: user:*@email.com'),
    ).toBeInTheDocument();
  });
  test('allows adding globs, case-sensitive', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    await userEvent.type(textfield!, '*@Email.com');
    const confirmButton = screen.queryByTestId('confirm-button');
    act(() => confirmButton!.click());
    // Check new member shown in list?
    expect(screen.getByText('user:*@Email.com')).toBeInTheDocument();
  });
});

describe('<GroupsFormList editable subgroups/>', () => {
  const mockGroup = createMockGroupIndividual('123', true, true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormList
          name="Subgroups"
          initialValues={mockGroup.nested as string[]}
          submitValues={() => {}}
        />
      </FakeContextProvider>,
    );
    await screen.findByTestId('groups-form-list');
  });
  test('shows error message on invalid subgroup', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'Subgroup');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(screen.getByText('Invalid Subgroups: Subgroup')).toBeInTheDocument();
  });
  test('shows error message on duplicate subgroup added', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen
      .getByTestId('add-textfield')
      .querySelector('textarea');
    await userEvent.type(textfield!, 'subgroup1');
    // Add member that already exists.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).toBeNull();
    // Check correct error message is shown.
    expect(
      screen.getByText('Invalid Subgroups: subgroup1'),
    ).toBeInTheDocument();
  });
  test('subgroups link correctly', async () => {
    const groups = mockGroup.nested;
    groups.forEach((group) => {
      expect(screen.getByText(group)).toBeInTheDocument();
      expect(screen.getByTestId(`${group}-link`)).toHaveAttribute(
        'href',
        `/ui/auth/groups/${group}`,
      );
    });
  });
});
