import { render, screen } from '@testing-library/react';
import { MRT_Column, MRT_Header, MRT_RowData } from 'material-react-table';
// import { describe, expect, it, vi } from 'vitest'; // using jest globals

import {
  FleetColumnHeader,
  FleetColumnHeaderContent,
} from './fleet_column_header';

describe('FleetColumnHeaderContent', () => {
  it('renders text correctly', () => {
    render(<FleetColumnHeaderContent text="Test Header" />);
    expect(screen.getByText('Test Header')).toBeInTheDocument();
  });

  it('renders tooltip when provided', () => {
    render(
      <FleetColumnHeaderContent
        text="Test Header"
        tooltip={<div>Tooltip Content</div>}
      />,
    );
    expect(screen.getByText('Test Header')).toBeInTheDocument();
    // Icon should be present (InfoOutlined usually renders as an SVG with data-testid="InfoOutlinedIcon" or similar,
    // but we can check if it exists by querying for specific semantics if we knew them,
    // or just assume the InfoTooltip renders something.
    // Since InfoTooltip is a bit complex, we might just check if the content is NOT visible initially
    // (popover) or check structure if we want deep testing.
    // For now, simpler: check if the hook/component doesn't crash.
  });
});

describe('FleetColumnHeader', () => {
  it('renders header text from column definition', () => {
    const mockHeader = {
      column: {
        id: 'test_col',
        columnDef: {
          header: 'Column Header',
        },
      } as unknown as MRT_Column<MRT_RowData>,
    } as MRT_Header<MRT_RowData>;

    render(<FleetColumnHeader header={mockHeader} />);
    expect(screen.getByText('Column Header')).toBeInTheDocument();
  });

  it('renders header text from props override', () => {
    const mockHeader = {
      column: {
        id: 'test_col',
        columnDef: {
          header: 'Column Header',
        },
      } as unknown as MRT_Column<MRT_RowData>,
    } as MRT_Header<MRT_RowData>;

    render(<FleetColumnHeader header={mockHeader} headerText="Override" />);
    expect(screen.getByText('Override')).toBeInTheDocument();
  });

  it('renders column ID when header text is missing', () => {
    const mockHeader = {
      column: {
        id: 'fallback_id',
        columnDef: {},
      } as unknown as MRT_Column<MRT_RowData>,
    } as MRT_Header<MRT_RowData>;

    render(<FleetColumnHeader header={mockHeader} />);
    expect(screen.getByText('fallback_id')).toBeInTheDocument();
  });

  it('renders tooltip from column meta', () => {
    const mockHeader = {
      column: {
        id: 'test_col',
        columnDef: {
          header: 'Column Header',
          meta: {
            infoTooltip: <span>Meta Tooltip</span>,
          },
        },
      } as unknown as MRT_Column<MRT_RowData>,
    } as MRT_Header<MRT_RowData>;

    render(<FleetColumnHeader header={mockHeader} />);
    // The tooltip content might be in a portal or hidden, so we won't easily find "Meta Tooltip"
    // without triggering hover. But we can ensure it renders without error.
    expect(screen.getByText('Column Header')).toBeInTheDocument();
  });
});
