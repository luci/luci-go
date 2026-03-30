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

import { Lock as LockIcon } from '@mui/icons-material';
import {
  Box,
  Checkbox,
  FormControl,
  ListSubheader,
  MenuItem,
  Select,
  SelectChangeEvent,
  Typography,
} from '@mui/material';
import {
  MaterialReactTable,
  MRT_Cell,
  MRT_ColumnDef,
  MRT_PaginationState,
  useMaterialReactTable,
} from 'material-react-table';
import { Dispatch, SetStateAction, useEffect, useMemo, useState } from 'react';

import { COMMON_MESSAGES, COMMON_MRT_CONFIG } from '@/crystal_ball/constants';
import {
  BreakdownSection,
  BreakdownTableConfig_BreakdownAggregation,
  breakdownTableConfig_BreakdownAggregationFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * The set of metric keys that might appear in breakdown data.
 * Used to distinguish metric columns from dimension columns.
 */
const BREAKDOWN_METRIC_KEYS = new Set(
  Object.keys(BreakdownTableConfig_BreakdownAggregation).filter(
    (key) => isNaN(Number(key)) && key !== 'BREAKDOWN_AGGREGATION_UNSPECIFIED',
  ),
);

/**
 * Labels for breakdown aggregations.
 */
const AGGREGATION_LABELS: Record<
  BreakdownTableConfig_BreakdownAggregation,
  string
> = {
  [BreakdownTableConfig_BreakdownAggregation.BREAKDOWN_AGGREGATION_UNSPECIFIED]:
    'UNKNOWN',
  [BreakdownTableConfig_BreakdownAggregation.COUNT]: 'COUNT',
  [BreakdownTableConfig_BreakdownAggregation.MIN]: 'MIN',
  [BreakdownTableConfig_BreakdownAggregation.MAX]: 'MAX',
  [BreakdownTableConfig_BreakdownAggregation.MEAN]: 'MEAN',
  [BreakdownTableConfig_BreakdownAggregation.P50]: 'P50',
  [BreakdownTableConfig_BreakdownAggregation.P75]: 'P75',
  [BreakdownTableConfig_BreakdownAggregation.P90]: 'P90',
  [BreakdownTableConfig_BreakdownAggregation.P99]: 'P99',
};

/**
 * The key used for the dimension column in the breakdown table.
 */
export const DIMENSION_COLUMN_KEY = 'dimension_value';

/**
 * Represents a single row of breakdown data, where keys are dimension names
 * or metric names, and values are either strings or numbers or null when missing.
 */
export type BreakdownRow = Record<string, string | number | null | undefined>;

interface InnerTableProps {
  /** The breakdown section (grouping dimension and its rows) to render. */
  section: BreakdownSection;
  /** The parent-hoisted pagination state. */
  pagination: MRT_PaginationState;
  /** State setter to update the parent-hoisted pagination state. */
  setPagination: Dispatch<SetStateAction<MRT_PaginationState>>;
}

interface TableCellProps {
  cell: MRT_Cell<BreakdownRow, string | number | null | undefined>;
}

const DimensionCell = ({ cell }: TableCellProps) => {
  const val = cell.getValue();
  const text = typeof val === 'string' || typeof val === 'number' ? val : '-';
  return (
    <Box
      sx={{
        display: 'inline-block',
        bgcolor: 'divider',
        p: 0.5,
        borderRadius: 1,
        fontFamily: 'monospace',
        fontSize: (theme) => theme.typography.body2.fontSize,
      }}
    >
      {text}
    </Box>
  );
};

const MetricCell = ({ cell }: TableCellProps) => {
  const val = cell.getValue();
  return (
    <Box
      sx={{
        fontFamily: 'monospace',
        textAlign: 'right',
      }}
    >
      {typeof val === 'number' ? val.toFixed(2) : (val ?? '-')}
    </Box>
  );
};

function InnerTable({
  section,
  pagination,
  setPagination,
}: InnerTableProps): React.ReactElement {
  const dimensionColumn = section.dimensionColumn ?? 'unknown';
  const data: BreakdownRow[] = useMemo(
    (): BreakdownRow[] => (section.rows ? [...section.rows] : []),
    [section.rows],
  );

  const dimensionKey = useMemo(() => {
    if (!data.length) return DIMENSION_COLUMN_KEY;
    const keys = Object.keys(data[0]);
    const nonMetricKeys = keys.filter((key) => !BREAKDOWN_METRIC_KEYS.has(key));
    return nonMetricKeys[0] ?? DIMENSION_COLUMN_KEY;
  }, [data]);

  const metricColumns = useMemo(() => {
    if (!data.length) return [];
    return Object.keys(data[0]).filter((key) => key !== dimensionKey);
  }, [data, dimensionKey]);

  const columns = useMemo<
    MRT_ColumnDef<BreakdownRow, number | string | null | undefined>[]
  >(() => {
    return [
      {
        accessorKey: dimensionKey,
        header: dimensionColumn.replace(/_/g, ' ').toUpperCase(),
        Cell: DimensionCell,
      },
      ...metricColumns.map(
        (
          col,
        ): MRT_ColumnDef<BreakdownRow, string | number | null | undefined> => ({
          accessorKey: col,
          header: col.toUpperCase(),
          muiTableHeadCellProps: {
            align: 'right',
          },
          Cell: MetricCell,
        }),
      ),
    ];
  }, [dimensionColumn, dimensionKey, metricColumns]);

  const countLabel =
    AGGREGATION_LABELS[BreakdownTableConfig_BreakdownAggregation.COUNT];
  const table = useMaterialReactTable<BreakdownRow>({
    ...COMMON_MRT_CONFIG,
    columns,
    data,
    state: { pagination },
    onPaginationChange: setPagination,
    initialState: {
      density: 'compact',
      sorting: metricColumns.includes(countLabel)
        ? [{ id: countLabel, desc: true }]
        : metricColumns.length > 0
          ? [{ id: metricColumns[0], desc: true }]
          : [],
    },
  });

  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)' }}>
      <MaterialReactTable table={table} />
    </Box>
  );
}

export interface BreakdownTableChartProps {
  sections: readonly BreakdownSection[];
  isLoading?: boolean;
  error?: Error | null;
  currentAggregations: readonly number[];
  onUpdateAggregations: (aggregations: number[]) => void;
  hasSeries: boolean;
}

/**
 * Renders the breakdown table chart visualization, including tabs for different
 * dimensions and a multi-select for aggregations.
 */
export function BreakdownTableChart({
  sections,
  isLoading,
  error,
  currentAggregations,
  onUpdateAggregations,
  hasSeries,
}: BreakdownTableChartProps): React.ReactElement {
  const [activeTab, setActiveTab] = useState(0);
  const [pagination, setPagination] = useState<MRT_PaginationState>({
    pageIndex: 0,
    pageSize: 10,
  });

  const [tempAggregations, setTempAggregations] = useState<number[]>([
    ...currentAggregations,
  ]);

  useEffect(() => {
    setTempAggregations([...currentAggregations]);
  }, [currentAggregations]);

  const handleAggregationsChange = (event: SelectChangeEvent<number[]>) => {
    const {
      target: { value },
    } = event;
    if (Array.isArray(value)) {
      setTempAggregations(value);
    }
  };

  const handleAggregationsClose = () => {
    const required = [
      BreakdownTableConfig_BreakdownAggregation.COUNT,
      BreakdownTableConfig_BreakdownAggregation.MIN,
      BreakdownTableConfig_BreakdownAggregation.MAX,
    ];
    const finalValues = Array.from(new Set([...required, ...tempAggregations]));
    onUpdateAggregations(finalValues);
  };

  const activeSection = sections[activeTab];

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        flexGrow: 1,
      }}
    >
      <Box
        sx={{
          p: 1.5,
          bgcolor: 'background.default',
          borderBottom: '1px solid',
          borderColor: 'divider',
          display: 'flex',
          alignItems: 'center',
          gap: 2,
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography
            variant="body2"
            sx={{ fontWeight: (theme) => theme.typography.fontWeightMedium }}
          >
            Breakdown by:
          </Typography>
          <FormControl size="small" variant="outlined" sx={{ minWidth: 160 }}>
            <Select
              id="dimension-select"
              value={String(activeTab)}
              onChange={(e: SelectChangeEvent<string>) => {
                setActiveTab(Number(e.target.value));
                setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
              inputProps={{ 'aria-label': 'Breakdown by category' }}
              sx={{
                bgcolor: 'background.paper',
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
                return (
                  sec.dimensionColumn?.replace(/_/g, ' ').toUpperCase() ||
                  'UNKNOWN'
                );
              }}
            >
              {sections.map((section, idx) => (
                <MenuItem
                  key={section.dimensionColumn ?? `UNKNOWN-${idx}`}
                  value={String(idx)}
                  sx={{
                    fontSize: (theme) => theme.typography.body2.fontSize,
                  }}
                >
                  {section.dimensionColumn?.replace(/_/g, ' ').toUpperCase() ||
                    'UNKNOWN'}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography
            variant="body2"
            sx={{ fontWeight: (theme) => theme.typography.fontWeightMedium }}
          >
            Aggregates:
          </Typography>
          <FormControl size="small" variant="outlined" sx={{ minWidth: 240 }}>
            <Select
              id="aggregations-select"
              multiple
              value={tempAggregations}
              onChange={handleAggregationsChange}
              onClose={handleAggregationsClose}
              inputProps={{ 'aria-label': 'Aggregates' }}
              sx={{
                bgcolor: 'background.paper',
                fontSize: (theme) => theme.typography.body2.fontSize,
                fontWeight: (theme) => theme.typography.body2.fontWeight,
                '& .MuiSelect-select': {
                  py: 0.75,
                },
              }}
              renderValue={(selected) => {
                if (!Array.isArray(selected)) {
                  return '';
                }
                return selected
                  .map((val) => {
                    const normalizedVal: BreakdownTableConfig_BreakdownAggregation =
                      typeof val === 'string'
                        ? breakdownTableConfig_BreakdownAggregationFromJSON(val)
                        : typeof val === 'number'
                          ? val
                          : BreakdownTableConfig_BreakdownAggregation.BREAKDOWN_AGGREGATION_UNSPECIFIED;
                    return AGGREGATION_LABELS[normalizedVal] ?? 'UNKNOWN';
                  })
                  .join(', ');
              }}
            >
              <ListSubheader
                sx={{
                  fontWeight: (theme) => theme.typography.fontWeightBold,
                  fontSize: (theme) => theme.typography.caption.fontSize,
                  lineHeight: (theme) => theme.typography.pxToRem(32),
                  color: 'text.secondary',
                  backgroundColor: 'background.paper',
                }}
              >
                SELECT COLUMNS
              </ListSubheader>
              {[
                BreakdownTableConfig_BreakdownAggregation.COUNT,
                BreakdownTableConfig_BreakdownAggregation.MIN,
                BreakdownTableConfig_BreakdownAggregation.MAX,
                BreakdownTableConfig_BreakdownAggregation.P50,
                BreakdownTableConfig_BreakdownAggregation.P75,
                BreakdownTableConfig_BreakdownAggregation.P90,
                BreakdownTableConfig_BreakdownAggregation.MEAN,
              ].map((agg) => {
                const isRequired = [
                  BreakdownTableConfig_BreakdownAggregation.COUNT,
                  BreakdownTableConfig_BreakdownAggregation.MIN,
                  BreakdownTableConfig_BreakdownAggregation.MAX,
                ].includes(agg);

                const label = AGGREGATION_LABELS[agg] ?? 'UNKNOWN';

                const isChecked =
                  isRequired ||
                  (Array.isArray(tempAggregations) &&
                    tempAggregations.includes(agg));

                return (
                  <MenuItem
                    key={agg}
                    value={agg}
                    disabled={isRequired}
                    sx={{
                      fontSize: (theme) => theme.typography.body2.fontSize,
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      py: 0.5,
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center' }}>
                      <Checkbox
                        checked={isChecked}
                        disabled={isRequired}
                        size="small"
                        sx={{ p: 0.5, mr: 1 }}
                      />
                      <span>{label}</span>
                    </Box>
                    {isRequired && (
                      <LockIcon
                        sx={{
                          fontSize: (theme) => theme.typography.body2.fontSize,
                          color: 'text.secondary',
                          ml: 0.5,
                        }}
                      />
                    )}
                  </MenuItem>
                );
              })}
            </Select>
          </FormControl>
        </Box>
      </Box>

      <Box sx={{ position: 'relative', minHeight: '100px', flexGrow: 1 }}>
        {isLoading && (
          <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
            Loading breakdown data...
          </Box>
        )}
        {error && (
          <Box sx={{ p: 2, color: 'error.main' }}>Error: {error.message}</Box>
        )}
        {!isLoading && !error && !sections.length && (
          <Box sx={{ p: 2 }}>
            <Typography variant="body2">
              {!hasSeries
                ? COMMON_MESSAGES.SERIES_REQUIRED
                : COMMON_MESSAGES.NO_DATA_AVAILABLE}
            </Typography>
          </Box>
        )}
        {!isLoading && !error && sections.length > 0 && (
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
        )}
      </Box>
    </Box>
  );
}
