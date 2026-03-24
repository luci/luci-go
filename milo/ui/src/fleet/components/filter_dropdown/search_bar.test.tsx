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

import { FilterCategory } from '../filters/use_filters';

import { SearchBar } from './search_bar';

const createMockFilterCategory = (
  value: string,
  label: string,
  isActive = true,
): FilterCategory =>
  ({
    value,
    label,
    isActive: () => isActive,
    renderChip: () => <div key={value}>{label}</div>,
    render: () => <div key={`menu-${value}`}>{label} Menu</div>,
    getChipLabel: () => label,
    clear: jest.fn(),
  }) as unknown as FilterCategory;

const TEST_SELECTED_OPTIONS: FilterCategory[] = [
  createMockFilterCategory('option1', 'Option 1'),
];

describe('SearchBar', () => {
  let onChangeDropdownOpen: jest.Mock;
  let onDropdownFocus: jest.Mock;
  let onChange: jest.Mock;
  let onChipEditApplied: jest.Mock;

  beforeEach(() => {
    onChangeDropdownOpen = jest.fn();
    onDropdownFocus = jest.fn();
    onChange = jest.fn();
    onChipEditApplied = jest.fn();
  });

  const renderComponent = (
    value = '',
    filterCategoryDatas = TEST_SELECTED_OPTIONS,
  ) => {
    return render(
      <SearchBar
        value={value}
        onChange={onChange}
        filterCategoryDatas={filterCategoryDatas}
        isDropdownOpen={false}
        onDropdownFocus={onDropdownFocus}
        onChangeDropdownOpen={onChangeDropdownOpen}
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

    // Clicking the input should open the dropdown
    const input = screen.getByPlaceholderText('Add a filter');
    await user.click(input);
    expect(onChangeDropdownOpen).toHaveBeenCalledWith(true);
  });
});
