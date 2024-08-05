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
import { createMockGroupIndividual } from '@/authdb/testing_tools/mocks/group_individual_mock';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { GroupsFormList } from './groups_form_list';
import { act } from 'react';

describe('<GroupsFormList editable/>', () => {
    const mockGroup = createMockGroupIndividual('123', true);
    beforeEach(async () => {
      render(
        <FakeContextProvider>
          <GroupsFormList name='Members' initialItems={mockGroup.members as string[]}/>
        </FakeContextProvider>,
      );
      await screen.findByTestId('groups-form-list');
  });
    test('displays items', async () => {
      // Check each member is displayed.
      for (let i = 0; i < mockGroup.members.length; i++) {
          expect(screen.getByText(mockGroup.members[i])).toBeInTheDocument();
      }
      // Check no remove button exists on readonly.
      expect(screen.queryAllByTestId('remove-button')).toHaveLength(0);
    });

  test('shows remove button on hover', async () => {
    // Simulate mouse enter event each row.
    for (let i = 0; i < mockGroup.members.length; i++) {
      const row = screen.getByTestId(`item-row-${mockGroup.members[i]}`);
      fireEvent.mouseEnter(row);
      expect(screen.getByTestId(`remove-button-${mockGroup.members[i]}`)).not.toBeNull();
    }
  })

  test('can remove members', async () => {
    // Simulate mouse enter event each row.
    for (let i = 0; i < mockGroup.members.length; i++) {
      const row = screen.getByTestId(`item-row-${mockGroup.members[i]}`);
      fireEvent.mouseEnter(row);
      const removeButton = screen.getByTestId(`remove-button-${mockGroup.members[i]}`)
      act(() => removeButton.click());
      expect(screen.queryByText(mockGroup.members[i])).toBeNull();
    }
  })

  test('shows add button', async () => {
    // Check that remove button exists for each member.
    const addButton = screen.queryByTestId('add-button');
    // Check add button appears on edit.
    expect(addButton).not.toBeNull();
  })

  test('can add members', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
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
    // Check new member shown in list?
    expect(screen.getByText('newMember@email.com')).toBeInTheDocument();
  })

  test('can add members with enter button', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember@email.com');
    expect(textfield!.value).toBe('newMember@email.com');
    // Press enter on textfield.
    fireEvent.keyDown(textfield!, {key: 'Enter', code: 'Enter', charCode: 13})
    // Check new member shown in list.
    expect(screen.getByText('newMember@email.com')).toBeInTheDocument();
  })

  test('shows error message on adding email with no @ symbol', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check correct error message is shown.
    expect(screen.getByText('Each member should be an email address.')).toBeInTheDocument();
  })

  test('shows error message on invalid email', async () => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember@email/com');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check correct error message is shown.
    expect(screen.getByText('Each member should be an email address.')).toBeInTheDocument();
  })

  test('hides textfield with clear button', async() => {
    // Click add button.
    const addButton = screen.queryByTestId('add-button');
    expect(addButton).not.toBeNull();
    act(() => addButton!.click());
    // Type in textfield.
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'newMember');
    // Click clear button.
    const clearButton = screen.queryByTestId('clear-button');
    expect(clearButton).not.toBeNull();
    act(() => clearButton!.click());
    // Check textfield is no longer shown.
    await screen.findByTestId('add-button');
    expect(textfield).not.toBeInTheDocument();
  })
});

describe('<GroupsFormList editable globs/>', () => {
  const mockGroup = createMockGroupIndividual('123', true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormList name='Globs' initialItems={mockGroup.members as string[]}/>
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
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, '.glob');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check correct error message is shown.
    expect(screen.getByText('Each glob should use at least one wildcard (i.e. *).')).toBeInTheDocument();
  })
});

describe('<GroupsFormList editable subgroups/>', () => {
  const mockGroup = createMockGroupIndividual('123', true);
  beforeEach(async () => {
    render(
      <FakeContextProvider>
        <GroupsFormList name='Subgroups' initialItems={mockGroup.members as string[]}/>
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
    const textfield = screen.getByTestId('add-textfield').querySelector('input');
    expect(textfield).toBeInTheDocument();
    await userEvent.type(textfield!, 'Subgroup');
    // Click confirm button.
    const confirmButton = screen.queryByTestId('confirm-button');
    expect(confirmButton).not.toBeNull();
    act(() => confirmButton!.click());
    // Check correct error message is shown.
    expect(screen.getByText('Invalid subgroup name.')).toBeInTheDocument();
  })
});