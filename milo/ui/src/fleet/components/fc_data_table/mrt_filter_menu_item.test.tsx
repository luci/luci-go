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

import { fireEvent, render, screen } from '@testing-library/react';

import { MRTFilterMenuItem_OLD } from './mrt_filter_menu_item';

// TODO: Update this mock to use the new unified filter bar components once OptionsMenuOld is fully removed.
jest.mock('@/fleet/components/filter_dropdown/options_menu_old', () => ({
  OptionsMenuOld: ({
    elements,
    selectedElements,
    flipOption,
  }: {
    elements: { el: { value: string; label: string } }[];
    selectedElements: Set<string>;
    flipOption: (val: string) => void;
  }) => (
    <div data-testid="mock-options-menu">
      {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
      {elements.map((x: any) => (
        <div key={x.el.value}>
          <input
            type="checkbox"
            checked={selectedElements.has(x.el.value)}
            onChange={() => flipOption(x.el.value)}
            aria-label={x.el.label}
          />
          <span>{x.el.label}</span>
        </div>
      ))}
    </div>
  ),
}));

describe('<MRTFilterMenuItem_OLD />', () => {
  const mockCloseMenu = jest.fn();
  const mockSetFilterValue = jest.fn();
  const mockGetFilterValue = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    mockGetFilterValue.mockReturnValue(undefined);
  });

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const createMockColumn = (options?: unknown[]): any => {
    return {
      id: 'test_column',
      columnDef: {
        header: 'Test Column',
        filterSelectOptions: options,
      },
      getFilterValue: mockGetFilterValue,
      setFilterValue: mockSetFilterValue,
    };
  };

  it('renders disabled when no filter options exist for multi-select', () => {
    const column = createMockColumn(undefined);
    column.columnDef.filterVariant = 'multi-select';
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).toHaveAttribute('aria-disabled', 'true');
  });

  it('renders enabled when filterVariant is range without options', () => {
    const column = createMockColumn(undefined);
    column.columnDef.filterVariant = 'range';
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).not.toHaveAttribute('aria-disabled', 'true');
  });

  it('renders enabled when filterVariant is date-range without options', () => {
    const column = createMockColumn(undefined);
    column.columnDef.filterVariant = 'date-range';
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).not.toHaveAttribute('aria-disabled', 'true');
  });

  it('renders enabled when filter options exist', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    const menuItem = screen.getByText('Filter').closest('li');
    expect(menuItem).not.toHaveAttribute('aria-disabled', 'true');
  });

  it('opens dropdown and displays options on click', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    // The options dropdown should be portal-rendered and visible
    expect(screen.getByText('option1')).toBeVisible();
    expect(screen.getByText('option2')).toBeVisible();
  });

  it('applies selected filters and closes menu', () => {
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    const option1 = screen.getByLabelText('option1');
    fireEvent.click(option1);

    const applyButton = screen.getByRole('button', { name: /apply/i });
    fireEvent.click(applyButton);

    expect(mockSetFilterValue).toHaveBeenCalledWith(['option1']);
    expect(mockCloseMenu).toHaveBeenCalled();
  });

  it('checks items that are already filtered', () => {
    mockGetFilterValue.mockReturnValue(['option1']);
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    const checkbox1 = screen.getByLabelText('option1') as HTMLInputElement;
    const checkbox2 = screen.getByLabelText('option2') as HTMLInputElement;

    expect(checkbox1.checked).toBe(true);
    expect(checkbox2.checked).toBe(false);
  });

  it('checks items that are already filtered (quoted case)', () => {
    mockGetFilterValue.mockReturnValue(['DEVICE_STATE_AVAILABLE']);
    const column = createMockColumn([
      { value: '"DEVICE_STATE_AVAILABLE"', label: 'DEVICE_STATE_AVAILABLE' },
    ]);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    const checkbox1 = screen.getByLabelText(
      'DEVICE_STATE_AVAILABLE',
    ) as HTMLInputElement;

    expect(checkbox1.checked).toBe(true);
  });

  it('resets selected filters back to undefined if none selected', () => {
    mockGetFilterValue.mockReturnValue(['option1']);
    const column = createMockColumn(['option1', 'option2']);
    render(<MRTFilterMenuItem_OLD column={column} closeMenu={mockCloseMenu} />);

    fireEvent.click(screen.getByText('Filter'));

    // Deselect option1
    const option1 = screen.getByLabelText('option1');
    fireEvent.click(option1);

    const applyButton = screen.getByRole('button', { name: /apply/i });
    fireEvent.click(applyButton);

    expect(mockSetFilterValue).toHaveBeenCalledWith(undefined);
    expect(mockCloseMenu).toHaveBeenCalled();
  });
});
