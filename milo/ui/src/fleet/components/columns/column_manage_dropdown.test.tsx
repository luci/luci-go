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

import { ColumnsManageDropDown } from './column_manage_dropdown';

jest.mock('../filter_dropdown/options_menu', () => ({
  OptionsMenu: ({ elements }: { elements: { el: { label: string } }[] }) => (
    <div data-testid="mock-options-menu">
      {elements.map((item) => (
        <div key={item.el.label} data-testid="menu-item-mock">
          {item.el.label}
        </div>
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

    const setAnchorEL = jest.fn();
    const onToggleColumn = jest.fn();

    // Render with a fake anchor element to force it open
    const anchorEl = document.createElement('div');

    render(
      <ColumnsManageDropDown
        anchorEl={anchorEl}
        setAnchorEL={setAnchorEL}
        allColumns={allColumns}
        visibleColumns={visibleColumns}
        onToggleColumn={onToggleColumn}
      />,
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
});
