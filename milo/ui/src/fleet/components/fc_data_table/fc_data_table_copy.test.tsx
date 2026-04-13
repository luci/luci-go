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
import { MRT_TableInstance } from 'material-react-table';

import { FCDataTableCopy } from './fc_data_table_copy';

jest.mock('../shortcut_provider', () => ({
  useShortcut: jest.fn(),
}));

describe('FCDataTableCopy', () => {
  const mockWriteText = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    // Mock navigator.clipboard
    Object.defineProperty(navigator, 'clipboard', {
      value: {
        writeText: mockWriteText,
      },
      writable: true,
    });
  });

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const createMockTable = (rows: unknown[], columns: any[]) => {
    const columnsWithAccessor = columns.map((col) => ({
      ...col,
      columnDef: {
        ...col.columnDef,
        accessorKey: col.columnDef.accessorKey || col.id,
      },
    }));
    return {
      getSelectedRowModel: () => ({ rows }),
      getVisibleLeafColumns: () => columnsWithAccessor,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as unknown as MRT_TableInstance<any>;
  };

  it('renders nothing when no rows are selected', () => {
    const table = createMockTable([], []);
    const { container } = render(<FCDataTableCopy table={table} />);

    // Returns <div /> if rows.length === 0
    expect(container.firstChild).toBeInstanceOf(HTMLDivElement);
    expect(container.firstChild).toBeEmptyDOMElement();
  });

  it('copies data to clipboard on button click', () => {
    const rows = [
      {
        id: '1',

        getValue: (id: string) => {
          if (id === 'id') return '1';
          if (id === 'name') return 'Device 1';
          return undefined;
        },
      },
    ];
    const columns = [
      { id: 'id', columnDef: { header: 'ID' } },
      { id: 'name', columnDef: { header: 'Name' } },
    ];

    const table = createMockTable(rows, columns);
    render(<FCDataTableCopy table={table} />);

    const button = screen.getByRole('button', { name: /copy selected rows/i });
    fireEvent.click(button);

    expect(mockWriteText).toHaveBeenCalledWith('ID\tName\n1\tDevice 1');
  });

  it('handles object values by stringifying them', () => {
    const rows = [
      {
        id: '1',
        getValue: (id: string) => {
          if (id === 'id') return '1';
          if (id === 'data') return { foo: 'bar' };
          return undefined;
        },
      },
    ];
    const columns = [
      { id: 'id', columnDef: { header: 'ID' } },
      { id: 'data', columnDef: { header: 'Data' } },
    ];

    const table = createMockTable(rows, columns);
    render(<FCDataTableCopy table={table} />);

    const button = screen.getByRole('button', { name: /copy selected rows/i });
    fireEvent.click(button);

    expect(mockWriteText).toHaveBeenCalledWith('ID\tData\n1\t{"foo":"bar"}');
  });

  it('uses column id if header is not a string', () => {
    const rows = [
      {
        id: '1',
        getValue: () => 'val',
      },
    ];
    const columns = [
      { id: 'custom_id', columnDef: { header: { type: 'div' } } }, // Mock a non-string header
    ];

    const table = createMockTable(rows, columns);
    render(<FCDataTableCopy table={table} />);

    const button = screen.getByRole('button', { name: /copy selected rows/i });
    fireEvent.click(button);

    expect(mockWriteText).toHaveBeenCalledWith('custom_id\nval');
  });

  it('sanitizes cell values by replacing tabs and newlines', () => {
    const rows = [
      {
        id: '1',
        getValue: (id: string) => {
          if (id === 'id') return '1';
          if (id === 'text') return 'value with\ttab and\nnewline';
          return undefined;
        },
      },
    ];
    const columns = [
      { id: 'id', columnDef: { header: 'ID' } },
      { id: 'text', columnDef: { header: 'Text' } },
    ];

    const table = createMockTable(rows, columns);
    render(<FCDataTableCopy table={table} />);

    const button = screen.getByRole('button', { name: /copy selected rows/i });
    fireEvent.click(button);

    expect(mockWriteText).toHaveBeenCalledWith(
      'ID\tText\n1\tvalue with tab and newline',
    );
  });
});
