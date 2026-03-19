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

import { AccessTime } from '@mui/icons-material';
import {
  Box,
  Button,
  Divider,
  MenuItem,
  Popover,
  Select,
  TextField,
  Typography,
} from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import React, { useMemo, useState } from 'react';

import { CUSTOMIZE_OPTION } from '@/common/components/time_range_selector/time_range_selector_utils';
import {
  GLOBAL_TIME_RANGE_COLUMN,
  GLOBAL_TIME_RANGE_FILTER_ID,
  DASHBOARD_DATE_FORMAT,
} from '@/crystal_ball/constants';
import {
  DashboardState,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  perfFilterDefault_FilterOperatorFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

interface DashboardTimeRangeSelectorProps {
  onApply: (state: DashboardState) => void;
  dashboardState: DashboardState;
}

export function DashboardTimeRangeSelector({
  dashboardState,
  onApply,
}: DashboardTimeRangeSelectorProps) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const timeFilter = dashboardState.dashboardContent?.globalFilters?.find(
    (f) => f.id === GLOBAL_TIME_RANGE_FILTER_ID,
  );

  const initialValues = timeFilter?.range?.defaultValue?.values ?? [];
  const initialOperator =
    timeFilter?.range?.defaultValue?.filterOperator !== undefined
      ? perfFilterDefault_FilterOperatorFromJSON(
          timeFilter.range.defaultValue.filterOperator,
        )
      : PerfFilterDefault_FilterOperator.IN_PAST;

  let initialNumber = '3';
  let initialUnit = 'd';

  if (
    initialOperator === PerfFilterDefault_FilterOperator.IN_PAST &&
    initialValues[0]
  ) {
    const match = initialValues[0].match(/^(\d+)([a-zA-Z])$/);
    if (match) {
      initialNumber = match[1];
      initialUnit = match[2];
    }
  }

  const [localNumber, setLocalNumber] = useState<string>(initialNumber);
  const [localUnit, setLocalUnit] = useState<string>(initialUnit);
  const [localStartTime, setLocalStartTime] = useState<DateTime | null>(null);
  const [localEndTime, setLocalEndTime] = useState<DateTime | null>(null);

  const { startTime, endTime, timeOption } = useMemo<{
    startTime: DateTime | null;
    endTime: DateTime | null;
    timeOption: string;
  }>(() => {
    if (!timeFilter?.range?.defaultValue?.values) {
      return { startTime: null, endTime: null, timeOption: '3d' };
    }
    const operator = perfFilterDefault_FilterOperatorFromJSON(
      timeFilter.range.defaultValue.filterOperator,
    );
    const values = timeFilter.range.defaultValue.values;

    if (operator === PerfFilterDefault_FilterOperator.IN_PAST) {
      return {
        startTime: null,
        endTime: null,
        timeOption: values[0] || '3d',
      };
    }

    const opt = values[2] || CUSTOMIZE_OPTION;
    return {
      startTime:
        values[0] && values[0] !== ''
          ? DateTime.fromISO(values[0]).toUTC()
          : null,
      endTime:
        values[1] && values[1] !== ''
          ? DateTime.fromISO(values[1]).toUTC()
          : null,
      timeOption: opt,
    };
  }, [timeFilter]);

  const updateTimeRange = (
    newStartTime: DateTime | null,
    newEndTime: DateTime | null,
    newOption: string,
  ) => {
    const newGlobalFilters = [
      ...(dashboardState.dashboardContent?.globalFilters ?? []).filter(
        (f) => f.id !== GLOBAL_TIME_RANGE_FILTER_ID,
      ),
    ];

    newGlobalFilters.push(
      PerfFilter.fromPartial({
        id: GLOBAL_TIME_RANGE_FILTER_ID,
        column: GLOBAL_TIME_RANGE_COLUMN,
        displayName: 'Time Range (UTC)',
        range: {
          defaultValue: {
            values:
              newOption === CUSTOMIZE_OPTION
                ? [
                    newStartTime ? (newStartTime.toISO() ?? '') : '',
                    newEndTime ? (newEndTime.toISO() ?? '') : '',
                    newOption,
                  ]
                : [newOption],
            filterOperator:
              newOption === CUSTOMIZE_OPTION
                ? PerfFilterDefault_FilterOperator.BETWEEN
                : PerfFilterDefault_FilterOperator.IN_PAST,
          },
        },
      }),
    );

    onApply({
      ...dashboardState,
      dashboardContent: {
        widgets: dashboardState.dashboardContent?.widgets ?? [],
        dataSpecs: dashboardState.dashboardContent?.dataSpecs ?? {},
        globalFilters: newGlobalFilters,
      },
    });
  };

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
    setLocalStartTime(startTime);
    setLocalEndTime(endTime);
    if (timeOption !== CUSTOMIZE_OPTION) {
      const match = timeOption.match(/^(\d+)([a-zA-Z])$/);
      if (match) {
        setLocalNumber(match[1]);
        setLocalUnit(match[2]);
      }
    }
  };

  const handleApplyRelative = () => {
    updateTimeRange(null, null, localNumber + localUnit);
    setAnchorEl(null);
  };

  const handleApplyAbsolute = () => {
    updateTimeRange(localStartTime, localEndTime, CUSTOMIZE_OPTION);
    setAnchorEl(null);
  };

  return (
    <Box>
      <Button
        variant="outlined"
        onClick={handleClick}
        sx={{ display: 'flex', gap: '7px' }}
        data-testid="time-button"
      >
        <AccessTime />
        {timeOption === CUSTOMIZE_OPTION ? (
          <>
            {startTime
              ? startTime.toFormat(DASHBOARD_DATE_FORMAT)
              : 'unspecified'}{' '}
            -{' '}
            {endTime ? endTime.toFormat(DASHBOARD_DATE_FORMAT) : 'unspecified'}
          </>
        ) : (
          (() => {
            const match = timeOption.match(/^(\d+)([a-zA-Z])$/);
            if (!match) return `Last ${localNumber} days`; // Fallback to local state if match fails
            const num = match[1];
            const unit = match[2];
            const map: Record<string, string> = {
              m: 'minute',
              h: 'hour',
              d: 'day',
              w: 'week',
            };
            const baseWord = map[unit] ?? 'day';
            const unitWord = num === '1' ? baseWord : `${baseWord}s`;
            return `Last ${num} ${unitWord}`;
          })()
        )}
      </Button>
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
      >
        <Box
          sx={{
            p: 2,
            display: 'flex',
            flexDirection: 'column',
            gap: 2,
            maxWidth: '350px',
          }}
        >
          <Typography variant="subtitle2">Relative Range</Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Typography>Last</Typography>
            <TextField
              type="number"
              size="small"
              value={localNumber}
              onChange={(e) => setLocalNumber(e.target.value)}
              sx={{ width: 80 }}
              inputProps={{ min: 1, step: 1 }}
            />
            <Select
              size="small"
              value={localUnit}
              onChange={(e) => setLocalUnit(e.target.value as string)}
            >
              <MenuItem value="m">minutes</MenuItem>
              <MenuItem value="h">hours</MenuItem>
              <MenuItem value="d">days</MenuItem>
              <MenuItem value="w">weeks</MenuItem>
            </Select>
            <Button variant="contained" onClick={handleApplyRelative}>
              Apply
            </Button>
          </Box>

          <Divider />

          <Typography variant="subtitle2">Custom Range (UTC)</Typography>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <DateTimePicker
              label="From (UTC)"
              timezone="UTC"
              value={localStartTime}
              onChange={(newValue) => setLocalStartTime(newValue)}
              slotProps={{ field: { clearable: true } }}
            />
            <DateTimePicker
              label="To (UTC)"
              timezone="UTC"
              value={localEndTime}
              onChange={(newValue) => setLocalEndTime(newValue)}
              slotProps={{ field: { clearable: true } }}
            />
            <Button variant="contained" onClick={handleApplyAbsolute}>
              Apply
            </Button>
          </Box>
        </Box>
      </Popover>
    </Box>
  );
}
