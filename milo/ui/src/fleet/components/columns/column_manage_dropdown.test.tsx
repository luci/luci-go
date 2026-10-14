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

import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ColumnsManageDropDown } from './column_manage_dropdown';

jest.mock('../filter_dropdown/options_menu', () => ({
  OptionsMenu: ({
    elements,
    selectedElements,
    flipOption,
  }: {
    elements: { el: { label: string; value: string } }[];
    selectedElements: Set<string>;
    flipOption: (value: string) => void;
  }) => (
    <div data-testid="mock-options-menu">
      {elements.map((item) => (
        <button
          key={item.el.value}
          type="button"
          data-testid="menu-item-mock"
          data-selected={selectedElements.has(item.el.value)}
          onClick={() => flipOption(item.el.value)}
        >
          {item.el.label}
        </button>
      ))}
    </div>
  ),
}));

describe('<ColumnsManageDropDown />', () => {
  it('should render columns partitioned by visible columns first', () => {
    const allColumns = [
      { id: 'col1', label: 'Column 1' },
      { id: 'col2', label: 'Column 2' },
      { id: 'col3', label: 'Column 3' },
    ];

    // We expect col2 and col3 to be at the top because they are visible,
    // and col1 to be at the bottom since it's hidden.
    const visibleColumns = ['col2', 'col3'];

    const setAnchorEl = jest.fn();
    const onToggleColumn = jest.fn();

    // Render with a fake anchor element to force it open
    const anchorEl = document.createElement('div');

    render(
      <FakeContextProvider>
        <ColumnsManageDropDown
          anchorEl={anchorEl}
          setAnchorEl={setAnchorEl}
          allColumns={allColumns}
          visibleColumns={visibleColumns}
          onToggleColumn={onToggleColumn}
        />
      </FakeContextProvider>,
    );

    // Look for checkboxes by labels to verify they render without crashing
    const col1 = screen.getByText('Column 1');
    const col2 = screen.getByText('Column 2');
    const col3 = screen.getByText('Column 3');

    expect(col1).toBeInTheDocument();
    expect(col2).toBeInTheDocument();
    expect(col3).toBeInTheDocument();

    // Since options are sorted and partitioned (visible first, then hidden),
    // we can check their DOM order. The parent list renders the items.
    const items = screen.getAllByTestId('menu-item-mock');

    // Note: Due to fuzzy sorting algorithm and partitioning,
    // we'll just check if both the tuple spreading and partition didn't break.
    // The items list length should be 3.
    expect(items).toHaveLength(3);

    expect(items[0]).toHaveTextContent('Column 2');
    expect(items[1]).toHaveTextContent('Column 3');
    expect(items[2]).toHaveTextContent('Column 1');
  });

  it('searches columns by humanized label and by underlying column ID fallback', () => {
    const allColumns = [
      { id: 'label-servo_state', label: 'Servo State' },
      { id: 'battery_level', label: 'Battery Level' },
    ];

    const setAnchorEl = jest.fn();
    const onToggleColumn = jest.fn();
    const anchorEl = document.createElement('div');

    render(
      <FakeContextProvider>
        <ColumnsManageDropDown
          anchorEl={anchorEl}
          setAnchorEl={setAnchorEl}
          allColumns={allColumns}
          visibleColumns={[]}
          onToggleColumn={onToggleColumn}
        />
      </FakeContextProvider>,
    );

    const searchInput = screen.getByPlaceholderText(/search/i);
    fireEvent.change(searchInput, { target: { value: 'servo_state' } });

    expect(screen.getByText('Servo State')).toBeInTheDocument();
    expect(screen.queryByText('Battery Level')).not.toBeInTheDocument();

    fireEvent.change(searchInput, { target: { value: 'Battery' } });
    expect(screen.getByText('Battery Level')).toBeInTheDocument();
    expect(screen.queryByText('Servo State')).not.toBeInTheDocument();
  });

  it('marks temporary/filtered columns with an asterisk, keeps hidden filtered columns in top partition, and allows toggling them', () => {
    const allColumns = [
      { id: 'col1', label: 'Column 1' },
      { id: 'col2', label: 'Column 2' },
      { id: 'col3', label: 'Column 3' },
    ];

    // col2 is visible and filtered; col3 is hidden (dismissed) but still has an active filter
    const visibleColumns = ['col2'];
    const temporaryColumns = ['col2', 'col3'];
    const onToggleColumn = jest.fn();
    const anchorEl = document.createElement('div');

    render(
      <FakeContextProvider>
        <ColumnsManageDropDown
          anchorEl={anchorEl}
          setAnchorEl={jest.fn()}
          allColumns={allColumns}
          visibleColumns={visibleColumns}
          temporaryColumns={temporaryColumns}
          onToggleColumn={onToggleColumn}
        />
      </FakeContextProvider>,
    );

    const items = screen.getAllByTestId('menu-item-mock');
    expect(items).toHaveLength(3);
    // Both col2 (visible) and col3 (hidden but filtered) stay in the top partition with ' *'
    expect(items[0]).toHaveTextContent('Column 2 *');
    expect(items[0]).toHaveAttribute('data-selected', 'true');
    expect(items[1]).toHaveTextContent('Column 3 *');
    expect(items[1]).toHaveAttribute('data-selected', 'false');
    expect(items[2]).toHaveTextContent('Column 1');
    expect(items[2]).toHaveAttribute('data-selected', 'false');

    fireEvent.click(items[0]);
    expect(onToggleColumn).toHaveBeenCalledWith('col2');

    fireEvent.click(items[1]);
    expect(onToggleColumn).toHaveBeenCalledWith('col3');
  });

  it('derives temporary selection and asterisk indicators from allColumns metadata when temporaryColumns prop is omitted', () => {
    const allColumns = [
      { id: 'col1', label: 'Column 1' },
      {
        id: 'col2',
        label: 'Column 2',
        isTemporary: true,
        isHighlighted: true,
      },
      { id: 'col3', label: 'Column 3', isHighlighted: true },
    ];

    const anchorEl = document.createElement('div');

    render(
      <FakeContextProvider>
        <ColumnsManageDropDown
          anchorEl={anchorEl}
          setAnchorEl={jest.fn()}
          allColumns={allColumns}
          visibleColumns={[]}
          onToggleColumn={jest.fn()}
        />
      </FakeContextProvider>,
    );

    const items = screen.getAllByTestId('menu-item-mock');
    expect(items).toHaveLength(3);
    expect(items[0]).toHaveTextContent('Column 2 *');
    expect(items[0]).toHaveAttribute('data-selected', 'true');
    expect(items[1]).toHaveTextContent('Column 3 *');
    expect(items[1]).toHaveAttribute('data-selected', 'false');
    expect(items[2]).toHaveTextContent('Column 1');
    expect(items[2]).toHaveAttribute('data-selected', 'false');
  });
});
