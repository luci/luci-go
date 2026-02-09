// Copyright 2026 The LUCI Authors.
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
import userEvent from '@testing-library/user-event';
import { forwardRef } from 'react';

import { FilterCategoryData } from './filter_dropdown';
import { SearchBar } from './search_bar';

const TEST_SELECTED_OPTIONS: FilterCategoryData<unknown>[] = [
  {
    value: 'option1',
    label: 'Option 1',
    type: 'string_list',
    optionsComponent: forwardRef(function optionsComponent() {
      return <div>Option Component</div>;
    }),
    optionsComponentProps: {},
    getChildrenSearchScore: () => 0,
  },
];

describe('SearchBar', () => {
  let onChangeDropdownOpen: jest.Mock;
  let onDropdownFocus: jest.Mock;
  let onChange: jest.Mock;
  let onChipDeleted: jest.Mock;
  let getLabel: jest.Mock;
  let onChipEditApplied: jest.Mock;

  beforeEach(() => {
    onChangeDropdownOpen = jest.fn();
    onDropdownFocus = jest.fn();
    onChange = jest.fn();
    onChipDeleted = jest.fn();
    getLabel = jest.fn((o) => o.label);
    onChipEditApplied = jest.fn();
  });

  const renderComponent = (
    value = '',
    selectedOptions = TEST_SELECTED_OPTIONS,
  ) => {
    return render(
      <SearchBar
        value={value}
        onChange={onChange}
        selectedOptions={selectedOptions}
        isDropdownOpen={false}
        onDropdownFocus={onDropdownFocus}
        onChangeDropdownOpen={onChangeDropdownOpen}
        onChipDeleted={onChipDeleted}
        getLabel={getLabel}
        onChipEditApplied={onChipEditApplied}
      />,
    );
  };

  it('should not propagate click from selected chip to the search bar', async () => {
    renderComponent();

    const user = userEvent.setup();

    const chipLabel = screen.getByText('Option 1');
    await user.click(chipLabel);

    onChangeDropdownOpen.mockClear();

    // Click inside the dropdown (e.g. the option component) should NOT open the dropdown
    const optionComponent = screen.getByText('Option Component');
    await user.click(optionComponent);

    expect(onChangeDropdownOpen).not.toHaveBeenCalled();

    // Clicking the input should open the dropdown
    const input = screen.getByPlaceholderText(
      'Add a filter (e.g. "dut1" or "state:ready")',
    );
    await user.click(input);
    expect(onChangeDropdownOpen).toHaveBeenCalledWith(true);
  });
});
