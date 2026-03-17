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

import {
  Box,
  Chip,
  FormControl,
  MenuItem,
  Select,
  SelectChangeEvent,
  Typography,
} from '@mui/material';
import { Theme } from '@mui/material/styles';
import {
  MaterialReactTable,
  useMaterialReactTable,
  MRT_ColumnDef,
  MRT_PaginationState,
  MRT_Cell,
} from 'material-react-table';
import { useMemo, useState, Dispatch, SetStateAction } from 'react';

import { COMMON_MRT_CONFIG } from '@/crystal_ball/constants';
import type {
  BreakdownSection,
  BreakdownTableData,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * The key used for the dimension column in the breakdown table.
 */
export const DIMENSION_COLUMN_KEY = 'dimension_value';

/**
 * Represents a single row of breakdown data, where keys are dimension names
 * or metric names, and values are either strings or numbers or null when missing.
 */
export type BreakdownRow = Record<string, string | number | null | undefined>;

/**
 * Props for the BreakdownTableWidget component.
 */
export interface BreakdownTableWidgetProps {
  /** The aggregated metrics data to display. */
  data: BreakdownTableData;
}

/**
 * Props for the internal table renderer.
 */
interface InnerTableProps {
  /** The breakdown section (grouping dimension and its rows) to render. */
  section: BreakdownSection;
  /** The parent-hoisted pagination state. */
  pagination: MRT_PaginationState;
  /** State setter to update the parent-hoisted pagination state. */
  setPagination: Dispatch<SetStateAction<MRT_PaginationState>>;
}

/**
 * Encapsulates the Material React Table logic specifically for rendering a single,
 * active breakdown section. Uses the Inner Component pattern to automatically reset
 * table-specific state (like sorting) when switching tables.
 */
function InnerTable({
  section,
  pagination,
  setPagination,
}: InnerTableProps): React.ReactElement {
  const dimensionColumn = section.dimensionColumn || 'unknown';
  const data: BreakdownRow[] = useMemo(
    (): BreakdownRow[] => [...section.rows],
    [section.rows],
  );

  const metricColumns = useMemo(() => {
    if (!data.length) return [];
    return Object.keys(data[0]).filter((key) => key !== DIMENSION_COLUMN_KEY);
  }, [data]);

  const columns = useMemo<
    MRT_ColumnDef<BreakdownRow, number | string | null | undefined>[]
  >(() => {
    const cols: MRT_ColumnDef<
      BreakdownRow,
      number | string | null | undefined
    >[] = [
      {
        accessorKey: DIMENSION_COLUMN_KEY,
        id: 'dimensionColumn',
        header: dimensionColumn.replace(/_/g, ' ').toUpperCase(),
        size: 50,
        muiTableHeadCellProps: {
          align: 'left',
        },
        muiTableBodyCellProps: {
          align: 'left',
        },
        Cell: DimensionCell,
      },
    ];

    metricColumns.forEach((metric) => {
      cols.push({
        accessorKey: metric,
        header: metric.toUpperCase(),
        size: 50,
        muiTableHeadCellProps: {
          align: 'right',
        },
        muiTableBodyCellProps: {
          align: 'right',
          sx: {
            fontFamily: 'monospace',
            fontWeight: (theme: Theme) => theme.typography.fontWeightMedium,
          },
        },
        Cell: MetricCell,
      });
    });

    return cols;
  }, [dimensionColumn, metricColumns]);

  const table = useMaterialReactTable({
    ...COMMON_MRT_CONFIG,
    columns,
    data,
    state: { pagination },
    onPaginationChange: setPagination,
    initialState: {
      density: 'compact',
      sorting: [{ id: 'COUNT', desc: true }],
    },
  });

  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)' }}>
      <MaterialReactTable table={table} />
    </Box>
  );
}

interface TableCellProps {
  cell: MRT_Cell<BreakdownRow, number | string | null | undefined>;
}

const DimensionCell = ({ cell }: TableCellProps) => (
  <Chip label={String(cell.getValue() ?? '')} variant="outlined" size="small" />
);

const MetricCell = ({ cell }: TableCellProps) => {
  const val = cell.getValue();
  return typeof val === 'number' ? val.toFixed(2) : String(val ?? '');
};

/**
 * A widget that displays performance breakdown data.
 */
export function BreakdownTableWidget({
  data,
}: BreakdownTableWidgetProps): React.ReactElement {
  const [activeTab, setActiveTab] = useState(0);
  const [pagination, setPagination] = useState<MRT_PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  });

  const sections = data?.sections || [];
  const activeSection = sections[activeTab];

  if (!sections.length) {
    return <Box sx={{ p: 2 }}>No data available.</Box>;
  }

  return (
    <Box
      sx={{
        width: '100%',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        minWidth: 0,
      }}
    >
      <Box
        sx={{
          py: 1,
          px: 1.5,
          borderBottom: '1px solid',
          borderColor: 'divider',
          bgcolor: (theme) => theme.palette.background.paper,
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
        }}
      >
        <Typography variant="body2" color="text.secondary" fontWeight="medium">
          Breakdown by:
        </Typography>
        <FormControl
          size="small"
          variant="outlined"
          sx={{ minWidth: 160, bgcolor: 'background.paper' }}
        >
          <Select
            id="dimension-select"
            value={String(activeTab)}
            onChange={(e: SelectChangeEvent<string>) => {
              setActiveTab(Number(e.target.value));
              setPagination((prev) => ({ ...prev, pageIndex: 0 }));
            }}
            inputProps={{ 'aria-label': 'Breakdown by category' }}
            sx={{
              fontSize: (theme) => theme.typography.body2.fontSize,
              fontWeight: (theme) => theme.typography.body2.fontWeight,
              '& .MuiSelect-select': {
                py: 0.75,
              },
            }}
            renderValue={(val: string) => {
              const sec = sections[Number(val)];
              if (!sec) {
                return 'UNKNOWN';
              }
              const result =
                sec.dimensionColumn?.replace(/_/g, ' ').toUpperCase() ||
                'UNKNOWN';
              return result;
            }}
          >
            {sections.map((section, idx) => {
              const dimLabel = section.dimensionColumn || `UNKNOWN-${idx}`;
              return (
                <MenuItem
                  key={dimLabel}
                  value={String(idx)}
                  sx={{ fontSize: (theme) => theme.typography.body2.fontSize }}
                >
                  {section.dimensionColumn?.replace(/_/g, ' ').toUpperCase() ||
                    'UNKNOWN'}
                </MenuItem>
              );
            })}
          </Select>
        </FormControl>
      </Box>
      <Box
        sx={{
          minWidth: 0,
          width: '100%',
          display: 'grid',
          gridTemplateColumns: 'minmax(0, 1fr)',
        }}
      >
        {activeSection && (
          <InnerTable
            key={activeSection.dimensionColumn ?? 'unknown-section'}
            section={activeSection}
            pagination={pagination}
            setPagination={setPagination}
          />
        )}
      </Box>
    </Box>
  );
}
