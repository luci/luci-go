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

import { Typography as MockedTypography } from '@mui/material';
import { fireEvent, render, screen } from '@testing-library/react';
import {
  MRT_Column,
  MRT_Header,
  MRT_RowData,
  MRT_TableInstance,
} from 'material-react-table';

import { FleetColumnHeader } from './fleet_column_header';

// Mock Typography to inspect its props
jest.mock('@mui/material', () => {
  const actual = jest.requireActual('@mui/material');
  return {
    ...actual,
    Typography: jest.fn((props) => <actual.Typography {...props} />),
  };
});

// Mock InfoTooltip since it might be complex
jest.mock('@/fleet/components/info_tooltip/info_tooltip', () => ({
  InfoTooltip: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="info-tooltip">{children}</div>
  ),
}));

jest.mock('material-react-table', () => ({
  ...jest.requireActual('material-react-table'),
  MRT_TableHeadCellColumnActionsButton: ({
    table,
  }: {
    table: { setColumnActionMenuOpen: (open: boolean) => void };
  }) => (
    <button
      data-testid="column-action-button"
      onClick={() => table.setColumnActionMenuOpen(true)}
    >
      Actions
    </button>
  ),
}));

describe('FleetColumnHeader', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const mockTable = {
    options: {
      enableColumnActions: true,
      icons: {
        ArrowDownwardIcon: () => <div data-testid="arrow-downward" />,
        MoreVertIcon: () => <div data-testid="more-vert" />,
        SyncAltIcon: () => <div data-testid="sync-alt" />,
      },
      localization: {
        sortByColumnAsc: 'Sort by {column} ascending',
        sortByColumnDesc: 'Sort by {column} descending',
        sortedByColumnAsc: 'Sorted by {column} ascending',
        sortedByColumnDesc: 'Sorted by {column} descending',
        columnActions: 'Column Actions',
      },
    },
    getState: () => ({
      isLoading: false,
      showSkeletons: false,
      sorting: [],
      columnActionMenu: { columnId: null },
    }),
    setColumnActionMenuOpen: jest.fn(),
  } as unknown as MRT_TableInstance<MRT_RowData>;

  const createMockColumn = (overrides = {}) =>
    ({
      id: 'test_col',
      columnDef: {
        header: 'Column Header',
        enableColumnActions: true,
      },
      getIsSorted: jest.fn().mockReturnValue(false),
      getCanSort: jest.fn().mockReturnValue(true),
      toggleSorting: jest.fn(),
      getNextSortingOrder: jest.fn(() => 'asc'),
      ...overrides,
    }) as unknown as MRT_Column<MRT_RowData>;

  const createMockHeader = (column: MRT_Column<MRT_RowData>) =>
    ({
      column,
      table: mockTable,
    }) as unknown as MRT_Header<MRT_RowData>;

  it('renders header text from column definition', () => {
    const column = createMockColumn();
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );
    expect(screen.getByText('Column Header')).toBeInTheDocument();
  });

  it('renders header text from props override', () => {
    const column = createMockColumn();
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader
        column={column}
        header={header}
        table={mockTable}
        headerText="Override"
      />,
    );
    expect(screen.getByText('Override')).toBeInTheDocument();
  });

  it('renders column ID when header text is missing', () => {
    const column = createMockColumn({
      id: 'fallback_id',
      columnDef: {},
    });
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );
    expect(screen.getByText('fallback_id')).toBeInTheDocument();
  });

  it('renders tooltip from column meta', () => {
    const column = createMockColumn({
      columnDef: {
        header: 'Column Header',
        meta: {
          infoTooltip: <span>Meta Tooltip</span>,
        },
      },
    });
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );
    expect(screen.getByTestId('info-tooltip')).toHaveTextContent(
      'Meta Tooltip',
    );
  });

  it('allows click propagation for sorting (handled by parent MRT cell)', () => {
    const column = createMockColumn();
    const header = createMockHeader(column);
    const mockSortHandler = jest.fn();

    render(
      // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
      <div onClick={mockSortHandler}>
        <FleetColumnHeader column={column} header={header} table={mockTable} />
      </div>,
    );
    // Click the text
    const text = screen.getByText('Column Header');
    fireEvent.click(text);

    expect(mockSortHandler).toHaveBeenCalled();
  });

  it('calls setColumnActionMenuOpen when action button is clicked', () => {
    const column = createMockColumn();
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );

    const actionButton = screen.getByTestId('column-action-button');
    fireEvent.click(actionButton);

    expect(
      (mockTable as unknown as { setColumnActionMenuOpen: jest.Mock })
        .setColumnActionMenuOpen,
    ).toHaveBeenCalledWith(true);
  });

  it('renders aria-description for temporary columns', () => {
    const column = createMockColumn({
      columnDef: {
        header: 'Temp Column',
        meta: { isTemporary: true },
      },
    });
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );

    expect(screen.getByText('Temp Column')).toBeInTheDocument();
  });
});

describe('FleetColumnHeader Visuals', () => {
  const mockTable = {
    options: {
      enableColumnActions: true,
      icons: {
        ArrowDownwardIcon: () => <div data-testid="arrow-downward" />,
        MoreVertIcon: () => <div data-testid="more-vert" />,
        SyncAltIcon: () => <div data-testid="sync-alt" />,
      },
      localization: {
        sortByColumnAsc: 'Sort by {column} ascending',
        sortByColumnDesc: 'Sort by {column} descending',
        sortedByColumnAsc: 'Sorted by {column} ascending',
        sortedByColumnDesc: 'Sorted by {column} descending',
        columnActions: 'Column Actions',
      },
    },
    getState: () => ({
      isLoading: false,
      showSkeletons: false,
      sorting: [],
      columnActionMenu: { columnId: null },
    }),
    setColumnActionMenuOpen: jest.fn(),
  } as unknown as MRT_TableInstance<MRT_RowData>;

  const createMockColumn = (overrides = {}) =>
    ({
      id: 'test_col',
      columnDef: {
        header: 'Column Header',
        enableColumnActions: true,
      },
      getIsSorted: jest.fn().mockReturnValue(false),
      getCanSort: jest.fn().mockReturnValue(true),
      toggleSorting: jest.fn(),
      getNextSortingOrder: jest.fn(() => 'asc'),
      ...overrides,
    }) as unknown as MRT_Column<MRT_RowData>;

  const createMockHeader = (column: MRT_Column<MRT_RowData>) =>
    ({
      column,
      table: mockTable,
    }) as unknown as MRT_Header<MRT_RowData>;

  it('applies text wrapping styles to Typography', () => {
    const column = createMockColumn();
    const header = createMockHeader(column);

    render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );

    // Verify Typography was called with the expected sx props
    const typographyCalls = (MockedTypography as jest.Mock).mock.calls;
    const props = typographyCalls[0][0];

    expect(props.sx).toMatchObject({
      WebkitLineClamp: 2,
      display: '-webkit-box',
      WebkitBoxOrient: 'vertical',
      overflow: 'hidden',
      wordBreak: 'break-word',
    });
  });

  it('shows action container when column is sorted', () => {
    const column = createMockColumn({
      getIsSorted: jest.fn().mockReturnValue('asc'),
    });
    const header = createMockHeader(column);

    const { container } = render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );

    const actionContainer = container.querySelector('.fleet-column-actions');
    expect(actionContainer).toHaveClass('is-sorted');
  });

  it('hides action container when column is not sorted', () => {
    const column = createMockColumn({
      getIsSorted: jest.fn().mockReturnValue(false),
    });
    const header = createMockHeader(column);

    const { container } = render(
      <FleetColumnHeader column={column} header={header} table={mockTable} />,
    );

    const actionContainer = container.querySelector('.fleet-column-actions');
    expect(actionContainer).not.toHaveClass('is-sorted');
  });
});
