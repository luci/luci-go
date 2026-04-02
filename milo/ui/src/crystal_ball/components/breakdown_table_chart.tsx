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
  Functions as FunctionsIcon,
  GroupWork as GroupWorkIcon,
  Lock as LockIcon,
} from '@mui/icons-material';
import {
  Alert,
  alpha,
  Box,
  Checkbox,
  CircularProgress,
  Divider,
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
  BackgroundAlpha,
  COMPACT_ICON_SX,
  COMPACT_SELECT_SX,
} from '@/crystal_ball/styles';
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

/**
 * Props for the RangeCell component.
 */
interface RangeCellProps {
  /** The data row containing min/max values. */
  row: BreakdownRow;
  /** The global minimum value for scaling the bar. */
  globalMin: number;
  /** The global maximum value for scaling the bar. */
  globalMax: number;
  /** The key for the minimum value in the row. */
  minKey?: string;
  /** The key for the maximum value in the row. */
  maxKey?: string;
}

/**
 * Renders a visual range bar indicating the min/max values relative to global bounds.
 */
const RangeCell = ({
  row,
  globalMin,
  globalMax,
  minKey: passedMinKey,
  maxKey: passedMaxKey,
}: RangeCellProps) => {
  const minKey =
    passedMinKey ?? Object.keys(row).find((k) => k.toUpperCase() === 'MIN');
  const maxKey =
    passedMaxKey ?? Object.keys(row).find((k) => k.toUpperCase() === 'MAX');
  const minVal = minKey ? Number(row[minKey]) : NaN;
  const maxVal = maxKey ? Number(row[maxKey]) : NaN;

  if (isNaN(minVal) || isNaN(maxVal)) return '-';

  const span = globalMax - globalMin;
  if (span === 0) return '-';

  const left = Math.min(Math.max(((minVal - globalMin) / span) * 100, 0), 100);
  const width = Math.min(Math.max(((maxVal - minVal) / span) * 100, 1), 100);

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        width: '100%',
        height: '100%',
        minWidth: 120,
      }}
    >
      <Box
        sx={{
          position: 'relative',
          width: '100%',
          height: 6,
          bgcolor: 'divider',
          borderRadius: 4,
        }}
      >
        <Box
          sx={{
            position: 'absolute',
            left: `${left}%`,
            width: `${width}%`,
            height: '100%',
            bgcolor: 'primary.main',
            borderRadius: 4,
          }}
        />
      </Box>
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
    const nonMetricKeys = keys.filter((key) => {
      const isMetric = Array.from(BREAKDOWN_METRIC_KEYS).some(
        (k) => k.toUpperCase() === key.toUpperCase(),
      );
      return !isMetric;
    });
    return nonMetricKeys[0] ?? DIMENSION_COLUMN_KEY;
  }, [data]);

  const metricColumns = useMemo(() => {
    if (!data.length) return [];
    return Object.keys(data[0]).filter((key) => key !== dimensionKey);
  }, [data, dimensionKey]);

  const { minKey, maxKey } = useMemo(() => {
    if (!data.length) return { minKey: undefined, maxKey: undefined };
    const keys = Object.keys(data[0]);
    return {
      minKey: keys.find((k) => k.toUpperCase() === 'MIN'),
      maxKey: keys.find((k) => k.toUpperCase() === 'MAX'),
    };
  }, [data]);

  const globalMinMax = useMemo(() => {
    let min = Infinity;
    let max = -Infinity;

    data.forEach((row) => {
      const rowMin = minKey ? Number(row[minKey]) : NaN;
      const rowMax = maxKey ? Number(row[maxKey]) : NaN;

      if (!isNaN(rowMin) && rowMin < min) min = rowMin;
      if (!isNaN(rowMax) && rowMax > max) max = rowMax;
    });
    return {
      min: min === Infinity ? 0 : min,
      max: max === -Infinity ? 0 : max,
    };
  }, [data, minKey, maxKey]);

  const columns = useMemo<
    MRT_ColumnDef<BreakdownRow, number | string | null | undefined>[]
  >(() => {
    return [
      {
        accessorKey: dimensionKey,
        header: dimensionColumn.replace(/_/g, ' ').toUpperCase(),
        Cell: DimensionCell,
      },
      {
        id: 'range_bar',
        header: 'RANGE',
        Cell: ({
          cell,
        }: {
          cell: MRT_Cell<BreakdownRow, string | number | null | undefined>;
        }) => (
          <RangeCell
            row={cell.row.original}
            globalMin={globalMinMax.min}
            globalMax={globalMinMax.max}
            minKey={minKey}
            maxKey={maxKey}
          />
        ),
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
  }, [
    dimensionColumn,
    dimensionKey,
    metricColumns,
    globalMinMax,
    minKey,
    maxKey,
  ]);

  const countLabel =
    AGGREGATION_LABELS[BreakdownTableConfig_BreakdownAggregation.COUNT];
  const table = useMaterialReactTable<BreakdownRow>({
    ...COMMON_MRT_CONFIG,
    columns,
    data,
    enableColumnResizing: true,
    state: { pagination },
    onPaginationChange: setPagination,
    renderEmptyRowsFallback: () => (
      <Box
        sx={{
          p: 2,
          textAlign: 'center',
          color: 'text.secondary',
        }}
      >
        <Typography variant="body1">{COMMON_MESSAGES.NO_DATA_FOUND}</Typography>
      </Box>
    ),
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

  const [tempAggregations, setTempAggregations] = useState<number[]>(() =>
    currentAggregations.map((a) =>
      typeof a === 'string'
        ? breakdownTableConfig_BreakdownAggregationFromJSON(a)
        : a,
    ),
  );

  useEffect(() => {
    setTempAggregations(
      currentAggregations.map((a) =>
        typeof a === 'string'
          ? breakdownTableConfig_BreakdownAggregationFromJSON(a)
          : a,
      ),
    );
  }, [currentAggregations]);

  const handleAggregationsChange = (event: SelectChangeEvent<number[]>) => {
    const {
      target: { value },
    } = event;
    if (Array.isArray(value)) {
      setTempAggregations(
        value.map((v) =>
          typeof v === 'string'
            ? breakdownTableConfig_BreakdownAggregationFromJSON(v)
            : v,
        ),
      );
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
          bgcolor: (theme) =>
            alpha(theme.palette.action.hover, BackgroundAlpha.LOW),
          borderBottom: '1px solid',
          borderColor: 'divider',
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
          pl: 2,
          pr: 0.5,
          py: 0.5,
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <GroupWorkIcon sx={COMPACT_ICON_SX} />
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {COMMON_MESSAGES.BREAKDOWN_BY}
            </Typography>
          </Box>
          <FormControl size="small" variant="outlined" sx={{ minWidth: 160 }}>
            <Select
              id="dimension-select"
              value={String(activeTab)}
              disabled={sections.length === 0}
              onChange={(e: SelectChangeEvent<string>) => {
                setActiveTab(Number(e.target.value));
                setPagination((prev) => ({ ...prev, pageIndex: 0 }));
              }}
              inputProps={{ 'aria-label': 'Breakdown by category' }}
              sx={COMPACT_SELECT_SX}
              renderValue={(val: string) => {
                const sec = sections[Number(val)];
                if (!sec) {
                  return sections.length === 0 ? 'N/A' : 'UNKNOWN';
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

        <Divider orientation="vertical" flexItem light />
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <FunctionsIcon sx={COMPACT_ICON_SX} />
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontWeight: (theme) => theme.typography.fontWeightBold,
                textTransform: 'uppercase',
                lineHeight: 1,
              }}
            >
              {COMMON_MESSAGES.AGGREGATES}
            </Typography>
          </Box>
          <FormControl size="small" variant="outlined" sx={{ minWidth: 240 }}>
            <Select
              id="aggregations-select"
              multiple
              value={tempAggregations}
              onChange={handleAggregationsChange}
              onClose={handleAggregationsClose}
              inputProps={{ 'aria-label': 'Aggregates' }}
              sx={COMPACT_SELECT_SX}
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
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              zIndex: 10,
              borderRadius: 1,
            }}
          >
            <CircularProgress />
          </Box>
        )}
        {error && (
          <Alert severity="error" sx={{ m: 2 }}>
            {error.message ?? COMMON_MESSAGES.UNKNOWN_ERROR}
          </Alert>
        )}
        {!isLoading && !error && !sections.length && (
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              height: '100%',
              minHeight: '300px',
            }}
          >
            <Typography variant="body1" sx={{ p: 2, color: 'text.secondary' }}>
              {!hasSeries
                ? COMMON_MESSAGES.METRIC_REQUIRED
                : COMMON_MESSAGES.NO_DATA_FOUND}
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
