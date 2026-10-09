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

import { DragIndicator, MoreVert, OpenInNew } from '@mui/icons-material';
import {
  Box,
  IconButton,
  LinearProgress,
  Tooltip,
  Typography,
} from '@mui/material';
import { useMemo, useRef, useState } from 'react';

import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { combineAipFiltersWithParentheses } from '@/fleet/utils/search_param';
import {
  HealthSlice,
  TrendlineGrouping,
  TrendlineMetricType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { BaselineChip } from './baseline_chip';
import { HealthSliceMenu } from './health_slice_menu';
import { getHealthStatus } from './health_status_utils';
import {
  getHourAlignedTimeWindow,
  RECENT_HOURS_WINDOW,
} from './time_window_utils';
import { useFleetAvailabilityTrends } from './use_fleet_availability_trends';

export interface HealthSliceItemProps {
  readonly slice: HealthSlice;
  readonly globalFilter?: string;
  readonly isSelected?: boolean;
  readonly isDefault?: boolean;
  readonly isDragging?: boolean;
  readonly isDragOver?: boolean;
  readonly isAnyDragging?: boolean;
  readonly onSelect?: (slice: HealthSlice) => void;
  readonly onEdit: (slice: HealthSlice) => void;
  readonly onDelete?: (slice: HealthSlice) => void;
  readonly onToggleDefault?: (slice: HealthSlice) => void;
  readonly onDragStart?: (slice: HealthSlice, e: React.DragEvent) => void;
  readonly onDragOver?: (slice: HealthSlice, e: React.DragEvent) => void;
  readonly onDragEnter?: (slice: HealthSlice, e: React.DragEvent) => void;
  readonly onDragLeave?: (slice: HealthSlice, e: React.DragEvent) => void;
  readonly onDrop?: (slice: HealthSlice, e: React.DragEvent) => void;
  readonly onDragEnd?: (e: React.DragEvent) => void;
  readonly testId?: string;
}

export const HealthSliceItem = ({
  slice,
  globalFilter,
  isSelected: explicitIsSelected,
  isDefault = false,
  isDragging = false,
  isDragOver = false,
  isAnyDragging = false,
  onSelect,
  onEdit,
  onDelete,
  onToggleDefault,
  onDragStart,
  onDragOver,
  onDragEnter,
  onDragLeave,
  onDrop,
  onDragEnd,
  testId,
}: HealthSliceItemProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const wasDraggedRef = useRef(false);
  const sliceFilter = slice.filter;
  const effectiveFilter = useMemo(
    () => combineAipFiltersWithParentheses(globalFilter, sliceFilter),
    [globalFilter, sliceFilter],
  );

  const isSelected = useMemo(() => {
    if (explicitIsSelected !== undefined) return explicitIsSelected;
    return (globalFilter ?? '').trim() === (sliceFilter ?? '').trim();
  }, [explicitIsSelected, globalFilter, sliceFilter]);

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
      data-testid={testId ?? `health-slice-item-${slice.id}`}
      draggable={Boolean(onDragStart)}
      onDragStart={(e) => {
        wasDraggedRef.current = true;
        e.dataTransfer.setData('text/plain', slice.id);
        e.dataTransfer.effectAllowed = 'move';
        onDragStart?.(slice, e);
      }}
      onDragOver={(e) => {
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
        onDragOver?.(slice, e);
      }}
      onDragEnter={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onDragEnter?.(slice, e);
      }}
      onDragLeave={(e) => {
        e.stopPropagation();
        onDragLeave?.(slice, e);
      }}
      onDrop={(e) => {
        e.preventDefault();
        e.stopPropagation();
        onDrop?.(slice, e);
      }}
      onDragEnd={(e) => {
        onDragEnd?.(e);
        setTimeout(() => {
          wasDraggedRef.current = false;
        }, 100);
      }}
      onClick={() => {
        if (wasDraggedRef.current) {
          wasDraggedRef.current = false;
          return;
        }
        onSelect?.(slice);
      }}
      sx={{
        position: 'relative',
        zIndex: isDragOver ? 10 : 1,
        p: 1.75,
        borderRadius: 2,
        bgcolor: isDragOver
          ? 'rgba(25, 118, 210, 0.08)'
          : isSelected
            ? 'rgba(25, 118, 210, 0.04)'
            : '#ffffff',
        border: '1px solid',
        borderColor: isDragOver
          ? 'primary.main'
          : isSelected
            ? 'primary.main'
            : 'divider',
        outline: isDragOver ? '2px dashed #1976d2' : 'none',
        outlineOffset: isDragOver ? '2px' : 0,
        opacity: isDragging ? 0.35 : 1,
        transform: 'none',
        boxShadow: isDragOver
          ? '0 4px 12px rgba(25, 118, 210, 0.25)'
          : isSelected
            ? '0 1px 4px rgba(25, 118, 210, 0.16)'
            : '0 1px 3px rgba(0,0,0,0.04)',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
        width: '100%',
        minHeight: 128,
        height: 128,
        boxSizing: 'border-box',
        cursor: onDragStart ? 'grab' : onSelect ? 'pointer' : 'default',
        '&:active': {
          cursor: onDragStart ? 'grabbing' : undefined,
        },
        transition: 'all 0.15s ease-in-out',
        '& *': isAnyDragging ? { pointerEvents: 'none' } : undefined,
        '&:hover .slice-card-actions': {
          opacity: 1,
        },
        '&:hover': onSelect
          ? {
              borderColor: isSelected ? 'primary.dark' : 'primary.main',
              bgcolor: isSelected ? 'rgba(25, 118, 210, 0.07)' : 'action.hover',
              boxShadow: isSelected
                ? '0 2px 6px rgba(25, 118, 210, 0.2)'
                : '0 2px 6px rgba(0,0,0,0.06)',
            }
          : {},
      }}
    >
      {/* Top Row: Slice Title with Drag Handle, BaselineChip & Action Controls */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'flex-start',
          justifyContent: 'space-between',
          gap: 0.75,
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            minWidth: 0,
            flex: 1,
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              minWidth: 0,
            }}
          >
            {Boolean(onDragStart) && (
              <DragIndicator
                sx={{
                  fontSize: 16,
                  color: isSelected ? 'primary.main' : 'text.disabled',
                  opacity: 0.6,
                  cursor: 'grab',
                  flexShrink: 0,
                  '&:hover': {
                    opacity: 1,
                    color: 'primary.main',
                  },
                }}
              />
            )}
            <Tooltip title={slice.name}>
              <Typography
                component="span"
                sx={{
                  fontWeight: 700,
                  fontSize: 14,
                  lineHeight: 1.3,
                  color: isSelected ? 'primary.dark' : 'text.primary',
                  userSelect: 'none',
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                  minWidth: 0,
                  flexShrink: 1,
                  '&:hover': {
                    textDecoration: 'underline',
                  },
                }}
              >
                {slice.name}
              </Typography>
            </Tooltip>

            <Box sx={{ flexShrink: 0, ml: '2px' }}>
              <BaselineChip
                isAvailability={isAvailability}
                testId={`slice-baseline-chip-${slice.id}`}
              />
            </Box>
          </Box>

          {isDefault && (
            <Typography
              variant="caption"
              data-testid={`slice-default-badge-${slice.id}`}
              sx={{
                fontSize: 11,
                fontWeight: 500,
                color: 'text.disabled',
                lineHeight: 1,
                mt: 0.5,
                pl: onDragStart ? 2.5 : 0,
              }}
            >
              Default view
            </Typography>
          )}
        </Box>

        <Box
          className="slice-card-actions"
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 0.25,
            flexShrink: 0,
            opacity: menuAnchorEl ? 1 : 0.45,
            transition: 'opacity 0.15s ease-in-out',
          }}
        >
          <IconButton
            component="a"
            href={deviceListUrl}
            target="_blank"
            rel="noreferrer"
            size="small"
            aria-label={`Open devices for ${slice.name}`}
            title="Open device list in new tab"
            onClick={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
            sx={{
              p: 0.25,
              color: 'text.secondary',
              '&:hover': { color: 'primary.main', bgcolor: 'action.hover' },
            }}
          >
            <OpenInNew sx={{ fontSize: 16 }} />
          </IconButton>

          <IconButton
            size="small"
            aria-label={`More options for ${slice.name}`}
            title="More options"
            onClick={(e) => {
              e.stopPropagation();
              setMenuAnchorEl(e.currentTarget);
            }}
            onMouseDown={(e) => e.stopPropagation()}
            sx={{
              p: 0.25,
              color: 'text.secondary',
              '&:hover': { color: 'primary.main', bgcolor: 'action.hover' },
            }}
          >
            <MoreVert sx={{ fontSize: 17 }} />
          </IconButton>

          <HealthSliceMenu
            slice={slice}
            isDefault={isDefault}
            anchorEl={menuAnchorEl}
            onClose={() => setMenuAnchorEl(null)}
            onEdit={onEdit}
            onToggleDefault={onToggleDefault}
            onDelete={onDelete}
          />
        </Box>
      </Box>

      {/* Bottom Section: Availability % & Status Label + Baseline Chip directly above Progress Bar */}
      <Box sx={{ width: '100%' }}>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            mb: 1,
            gap: 1,
          }}
        >
          <Box
            sx={{
              display: 'inline-flex',
              alignItems: 'baseline',
              gap: 0.75,
              minWidth: 0,
            }}
          >
            <Typography
              variant="h5"
              data-testid={`slice-pct-${slice.id}`}
              sx={{
                fontWeight: 800,
                lineHeight: 1,
                fontSize: 22,
                color:
                  healthMeta !== undefined
                    ? `${healthMeta.color}.main`
                    : 'text.secondary',
              }}
            >
              {pct !== undefined ? `${pct}%` : isLoading ? '...' : '—'}
            </Typography>
            <Typography
              variant="caption"
              sx={{
                fontWeight: 600,
                color: 'text.secondary',
                whiteSpace: 'nowrap',
              }}
            >
              {isAvailability ? 'Availability' : 'Health'}
            </Typography>
          </Box>

          <Typography
            variant="caption"
            sx={{
              fontWeight: 700,
              fontSize: 12,
              color:
                healthMeta !== undefined
                  ? `${healthMeta.color}.main`
                  : 'text.secondary',
              whiteSpace: 'nowrap',
            }}
          >
            {pct === undefined
              ? isLoading
                ? 'Loading...'
                : 'No data'
              : (healthMeta?.status ?? 'No data')}
          </Typography>
        </Box>
        <LinearProgress
          variant={isLoading ? 'indeterminate' : 'determinate'}
          value={Math.min(100, pct ?? 0)}
          color={healthMeta?.color ?? 'primary'}
          sx={{ height: 4, borderRadius: 2 }}
        />
      </Box>
    </Box>
  );
};
