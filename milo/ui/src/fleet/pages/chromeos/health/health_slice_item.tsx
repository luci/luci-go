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

import { Edit, OpenInNew } from '@mui/icons-material';
import {
  Box,
  Chip,
  IconButton,
  LinearProgress,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo } from 'react';

import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { combineAipFiltersWithParentheses } from '@/fleet/utils/search_param';
import {
  HealthSlice,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { BaselineChip } from './baseline_chip';
import { getHealthStatus } from './health_status_utils';
import {
  getHourAlignedTimeWindow,
  RECENT_HOURS_WINDOW,
} from './time_window_utils';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

export interface HealthSliceItemProps {
  readonly slice: HealthSlice;
  readonly globalFilter?: string;
  readonly onSelect?: (slice: HealthSlice) => void;
  readonly onEdit: (slice: HealthSlice) => void;
}

export const HealthSliceItem = ({
  slice,
  globalFilter,
  onSelect,
  onEdit,
}: HealthSliceItemProps) => {
  const sliceFilter = slice.filter;
  const effectiveFilter = useMemo(
    () => combineAipFiltersWithParentheses(globalFilter, sliceFilter),
    [globalFilter, sliceFilter],
  );

  const isSelected = useMemo(() => {
    if (!globalFilter || !sliceFilter) return false;
    return globalFilter.trim() === sliceFilter.trim();
  }, [globalFilter, sliceFilter]);

  const queryRequest = useMemo(() => {
    const { startTime, endTime } =
      getHourAlignedTimeWindow(RECENT_HOURS_WINDOW);
    return {
      grouping: TrendlineGrouping.GROUP_BY_OVERALL,
      startTime,
      endTime,
      filter: effectiveFilter,
    };
  }, [effectiveFilter]);

  const { data: trendsData, isLoading } =
    useFleetAvailabilityTrends(queryRequest);

  const points = useMemo(
    () => trendsData?.series?.[0]?.points ?? [],
    [trendsData?.series],
  );

  // Take freshest point in trendline.
  const activePoint = useMemo(
    () =>
      points.length > 0
        ? points.reduce((freshest, point) =>
            (point.timestamp ?? '') > (freshest.timestamp ?? '')
              ? point
              : freshest,
          )
        : undefined,
    [points],
  );

  const pct = useMemo(() => {
    if (!activePoint) return undefined;
    return Math.max(0, Math.round(activePoint.value * 100));
  }, [activePoint]);

  const isAvailability =
    trendsData?.metricType === TrendlineMetricType.AVAILABILITY;

  const healthMeta = pct !== undefined ? getHealthStatus(pct) : undefined;

  const deviceListUrl = `/ui/fleet/p/chromeos/devices${
    effectiveFilter
      ? `?${FILTERS_PARAM_KEY}=${encodeURIComponent(effectiveFilter)}`
      : ''
  }`;

  return (
    <Box
      data-testid={`health-slice-item-${slice.id}`}
      onClick={() => onSelect?.(slice)}
      sx={{
        p: 2,
        borderRadius: 2,
        bgcolor: isSelected ? 'action.selected' : '#ffffff',
        border: '1px solid',
        borderColor: isSelected ? 'primary.main' : 'divider',
        boxShadow: isSelected
          ? '0 0 0 1px #1976d2, 0 1px 3px rgba(0,0,0,0.08)'
          : '0 1px 2px rgba(0,0,0,0.04)',
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
        cursor: onSelect ? 'pointer' : 'default',
        transition: 'all 0.15s ease-in-out',
        '&:hover': onSelect
          ? {
              borderColor: 'primary.main',
              boxShadow: '0 2px 6px rgba(0,0,0,0.08)',
            }
          : {},
      }}
    >
      {/* Slice Title & Action Buttons */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 0.5,
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Typography
            component="span"
            sx={{
              fontWeight: 'bold',
              fontSize: 14,
              color: 'primary.main',
              userSelect: 'none',
              '&:hover': {
                textDecoration: 'underline',
              },
            }}
          >
            {slice.name}
          </Typography>
          <Tooltip title="Open device list in new tab">
            <IconButton
              component="a"
              href={deviceListUrl}
              target="_blank"
              rel="noreferrer"
              size="small"
              aria-label={`Open devices for ${slice.name}`}
              onClick={(e) => e.stopPropagation()}
              sx={{
                p: 0.3,
                color: 'text.secondary',
                '&:hover': { color: 'primary.main' },
              }}
            >
              <OpenInNew sx={{ fontSize: 15 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Edit slice name and filters">
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                onEdit(slice);
              }}
              aria-label={`Edit ${slice.name}`}
              sx={{
                p: 0.3,
                color: 'text.secondary',
                '&:hover': { color: 'primary.main' },
              }}
            >
              <Edit sx={{ fontSize: 15 }} />
            </IconButton>
          </Tooltip>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <BaselineChip
            isAvailability={isAvailability}
            testId={`slice-baseline-chip-${slice.id}`}
          />
          <Chip
            size="small"
            label={
              pct === undefined
                ? isLoading
                  ? 'LOADING'
                  : 'NO DATA'
                : (healthMeta?.status.toUpperCase() ?? 'NO DATA')
            }
            color={healthMeta?.color ?? 'default'}
            sx={{
              fontWeight: 'bold',
              height: 20,
              fontSize: 10,
            }}
          />
        </Box>
      </Box>

      {/* Availability / Health Percentage */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'baseline',
          gap: 1,
          mt: 0.5,
        }}
      >
        <Typography
          variant="h4"
          data-testid={`slice-pct-${slice.id}`}
          sx={{
            fontWeight: 'bold',
            lineHeight: 1,
            color:
              healthMeta !== undefined
                ? `${healthMeta.color}.main`
                : 'text.secondary',
          }}
        >
          {pct !== undefined ? `${pct}%` : isLoading ? '...' : '—'}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {isAvailability ? 'Availability' : 'Health'}
        </Typography>
      </Box>

      <LinearProgress
        variant={isLoading ? 'indeterminate' : 'determinate'}
        value={Math.min(100, pct ?? 0)}
        color={healthMeta?.color ?? 'primary'}
        sx={{ height: 6, borderRadius: 3, my: 0.5 }}
      />
    </Box>
  );
};
