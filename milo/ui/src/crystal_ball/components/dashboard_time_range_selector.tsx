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
  SelectChangeEvent,
  TextField,
  Typography,
} from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import React, { useMemo, useState } from 'react';

import { CUSTOMIZE_OPTION } from '@/common/components/time_range_selector/time_range_selector_utils';
import {
  COMMON_MESSAGES,
  DASHBOARD_DATE_FORMAT,
  GLOBAL_TIME_RANGE_COLUMN,
  GLOBAL_TIME_RANGE_FILTER_ID,
  GLOBAL_TIME_RANGE_OPTION_DEFAULT,
} from '@/crystal_ball/constants';
import { COMPACT_SELECT_SX, COMPACT_TEXTFIELD_SX } from '@/crystal_ball/styles';
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
  const [startTimeError, setStartTimeError] = useState<string | null>(null);
  const [endTimeError, setEndTimeError] = useState<string | null>(null);

  const { startTime, endTime, timeOption } = useMemo<{
    startTime: DateTime | null;
    endTime: DateTime | null;
    timeOption: string;
  }>(() => {
    if (!timeFilter?.range?.defaultValue?.values) {
      return {
        startTime: null,
        endTime: null,
        timeOption: GLOBAL_TIME_RANGE_OPTION_DEFAULT,
      };
    }
    const operator = perfFilterDefault_FilterOperatorFromJSON(
      timeFilter.range.defaultValue.filterOperator,
    );
    const values = timeFilter.range.defaultValue.values;

    if (operator === PerfFilterDefault_FilterOperator.IN_PAST) {
      return {
        startTime: null,
        endTime: null,
        timeOption: values[0] ?? GLOBAL_TIME_RANGE_OPTION_DEFAULT,
      };
    }

    const opt = values[2] ?? CUSTOMIZE_OPTION;
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
        displayName: COMMON_MESSAGES.TIME_RANGE_UTC,
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
    setStartTimeError(null);
    setEndTimeError(null);
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
    let isValid = true;
    setStartTimeError(null);
    setEndTimeError(null);

    if (!localStartTime) {
      setStartTimeError(COMMON_MESSAGES.FROM_DATE_REQUIRED);
      isValid = false;
    }
    if (!localEndTime) {
      setEndTimeError(COMMON_MESSAGES.TO_DATE_REQUIRED);
      isValid = false;
    }
    if (localStartTime && localEndTime && localStartTime > localEndTime) {
      setStartTimeError(COMMON_MESSAGES.FROM_DATE_MUST_BE_BEFORE_TO);
      setEndTimeError(COMMON_MESSAGES.TO_DATE_MUST_BE_AFTER_FROM);
      isValid = false;
    }

    if (isValid) {
      updateTimeRange(localStartTime, localEndTime, CUSTOMIZE_OPTION);
      setAnchorEl(null);
    }
  };

  return (
    <Box>
      <Button
        variant="outlined"
        onClick={handleClick}
        data-testid="time-button"
        startIcon={<AccessTime />}
      >
        {timeOption === CUSTOMIZE_OPTION ? (
          <>
            {startTime
              ? startTime.toFormat(DASHBOARD_DATE_FORMAT)
              : COMMON_MESSAGES.UNSPECIFIED}{' '}
            -{' '}
            {endTime
              ? endTime.toFormat(DASHBOARD_DATE_FORMAT)
              : COMMON_MESSAGES.UNSPECIFIED}
          </>
        ) : (
          (() => {
            const match = timeOption.match(/^(\d+)([a-zA-Z])$/);
            if (!match)
              return `${COMMON_MESSAGES.LAST} ${localNumber} ${COMMON_MESSAGES.DAYS}`; // Fallback to local state if match fails
            const num = match[1];
            const unit = match[2];
            const map: Record<string, COMMON_MESSAGES> = {
              m: COMMON_MESSAGES.MINUTE_ONE,
              h: COMMON_MESSAGES.HOUR_ONE,
              d: COMMON_MESSAGES.DAY_ONE,
              w: COMMON_MESSAGES.WEEK_ONE,
            };
            const baseWord = map[unit] ?? COMMON_MESSAGES.DAY_ONE;
            const unitWord = num === '1' ? baseWord : `${baseWord}s`;
            return `${COMMON_MESSAGES.LAST} ${num} ${unitWord}`;
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
          <Typography variant="subtitle2">
            {COMMON_MESSAGES.RELATIVE_RANGE}
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Typography>{COMMON_MESSAGES.LAST}</Typography>
            <TextField
              type="number"
              size="small"
              value={localNumber}
              onChange={(e) => setLocalNumber(e.target.value)}
              sx={COMPACT_TEXTFIELD_SX}
              inputProps={{ min: 1, step: 1 }}
            />
            <Select
              size="small"
              value={localUnit}
              onChange={(e: SelectChangeEvent<string>) =>
                setLocalUnit(e.target.value)
              }
              sx={COMPACT_SELECT_SX}
            >
              <MenuItem value="m">{COMMON_MESSAGES.MINUTES}</MenuItem>
              <MenuItem value="h">{COMMON_MESSAGES.HOURS}</MenuItem>
              <MenuItem value="d">{COMMON_MESSAGES.DAYS}</MenuItem>
              <MenuItem value="w">{COMMON_MESSAGES.WEEKS}</MenuItem>
            </Select>
            <Button variant="contained" onClick={handleApplyRelative}>
              {COMMON_MESSAGES.APPLY}
            </Button>
          </Box>

          <Divider />

          <Typography variant="subtitle2">
            {COMMON_MESSAGES.CUSTOM_RANGE_UTC}
          </Typography>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <DateTimePicker
              label={COMMON_MESSAGES.FROM_UTC}
              timezone="UTC"
              value={localStartTime}
              onChange={(newValue) => {
                setLocalStartTime(newValue);
                if (startTimeError) setStartTimeError(null);
              }}
              slotProps={{
                textField: {
                  error: Boolean(startTimeError),
                  helperText: startTimeError,
                },
                field: { clearable: true },
              }}
            />
            <DateTimePicker
              label={COMMON_MESSAGES.TO_UTC}
              timezone="UTC"
              value={localEndTime}
              onChange={(newValue) => {
                setLocalEndTime(newValue);
                if (endTimeError) setEndTimeError(null);
              }}
              slotProps={{
                textField: {
                  error: Boolean(endTimeError),
                  helperText: endTimeError,
                },
                field: { clearable: true },
              }}
            />
            <Button variant="contained" onClick={handleApplyAbsolute}>
              {COMMON_MESSAGES.APPLY}
            </Button>
          </Box>
        </Box>
      </Popover>
    </Box>
  );
}
