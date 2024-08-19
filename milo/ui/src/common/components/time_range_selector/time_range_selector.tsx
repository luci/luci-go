// Copyright 2024 The LUCI Authors.
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
import { Box, Button, Menu, MenuItem, Popover } from '@mui/material';
import { DateTimePicker } from '@mui/x-date-pickers/DateTimePicker';
import { DateTime } from 'luxon';
import React, { useState } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Result } from '@/generic_libs/types';

import {
  getSelectedOption,
  getStartEndTime,
  CUSTOMIZE_OPTION,
  MenuOption,
  RELATIVE_TIME_OPTIONS,
  CUSTOMIZE_OPTION_DISPLAY_TEXT,
  timeRangeParamUpdater,
  optionParamUpdater,
  END_TIME_KEY,
  START_TIME_KEY,
} from './time_range_selector_utils';

export interface TimeRangeSelectorProps {
  /**
   * A function that returns error message when the given startTime and endTime is an invalid
   * time range. Return null or empty string means no error.
   */
  readonly validateCustomizeTimeRange?: (
    startTime: DateTime,
    endTime: DateTime,
  ) => Result<'', string>;
  readonly disableFuture?: boolean;
}

/**
 * TimeRangeSelector implement a component that supports absolute and relative time range selection.
 * It saves the selected time range option to search params. It also read the search params to set existing selection.
 * Parent can use getAbsoluteStartEndTime function to obtain the selected start and end time.
 */
export function TimeRangeSelector({
  validateCustomizeTimeRange,
  disableFuture = false,
}: TimeRangeSelectorProps) {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [pickerAnchorEl, setPickerAnchorElPicker] =
    useState<null | HTMLElement>(null);
  const selectedOption = getSelectedOption(searchParams);
  const { startTime, endTime } = getStartEndTime(searchParams);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setMenuAnchorEl(event.currentTarget);
  };

  // TODO: Currently, clicking the customise option will immediately change the selection.
  // A better user experience is to only update the selection
  // after user puts in a valid time range and clicks confirm.
  const handleMenuItemClick = (
    option: MenuOption,
    event: React.MouseEvent<HTMLLIElement>,
  ) => {
    const prevOption = getSelectedOption(searchParams);
    // Update search params
    setSearchParams(optionParamUpdater(option));
    if (option === CUSTOMIZE_OPTION) {
      if (prevOption !== CUSTOMIZE_OPTION) {
        // Prefill time range with previouly selected relative range.
        const endTime = DateTime.now();
        const startTime = endTime.minus(
          RELATIVE_TIME_OPTIONS[prevOption].duration,
        );
        setSearchParams(timeRangeParamUpdater(START_TIME_KEY, startTime));
        setSearchParams(timeRangeParamUpdater(END_TIME_KEY, endTime));
      }
      setPickerAnchorElPicker(event.currentTarget);
      return;
    }
    setSearchParams(timeRangeParamUpdater(START_TIME_KEY, null));
    setSearchParams(timeRangeParamUpdater(END_TIME_KEY, null));
    // Close the main menu
    setMenuAnchorEl(null);
  };

  const handlePopoverClose = () => {
    setPickerAnchorElPicker(null);
    setMenuAnchorEl(null);
  };

  const startTimeErrMsg = (() => {
    if (selectedOption !== CUSTOMIZE_OPTION) {
      return null;
    }
    if (!startTime) {
      return 'unspecified';
    }
    // Show the time range error only under start time to avoid displaying duplicated error messages.
    return endTime && validateCustomizeTimeRange
      ? validateCustomizeTimeRange(startTime, endTime).value
      : null;
  })();

  const endTimeErrMsg =
    selectedOption === CUSTOMIZE_OPTION && !endTime ? 'unspecified' : null;

  return (
    <div>
      <Button
        variant="outlined"
        onClick={handleClick}
        sx={{ display: 'flex', gap: '7px' }}
        data-testid="time-button"
        color={!startTimeErrMsg && !endTimeErrMsg ? 'primary' : 'error'}
      >
        <AccessTime />
        {selectedOption === CUSTOMIZE_OPTION ? (
          <>
            {startTime ? startTime.toFormat('yyyy-MM-dd HH:mm') : 'unspecified'}{' '}
            - {endTime ? endTime.toFormat('yyyy-MM-dd HH:mm') : 'unspecified'}
          </>
        ) : (
          RELATIVE_TIME_OPTIONS[selectedOption].displayText
        )}
      </Button>
      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={() => setMenuAnchorEl(null)}
      >
        {Object.entries(RELATIVE_TIME_OPTIONS).map(([key, value]) => (
          <MenuItem
            key={key}
            data-testid={key}
            onClick={(e) => handleMenuItemClick(key as MenuOption, e)}
          >
            {value.displayText}
          </MenuItem>
        ))}
        <MenuItem
          data-testid={CUSTOMIZE_OPTION}
          onClick={(e) => handleMenuItemClick(CUSTOMIZE_OPTION, e)}
        >
          {CUSTOMIZE_OPTION_DISPLAY_TEXT}
        </MenuItem>
      </Menu>
      <Popover
        open={Boolean(pickerAnchorEl)}
        anchorEl={pickerAnchorEl}
        onClose={handlePopoverClose}
        anchorOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
      >
        <Box
          sx={{
            padding: '5px',
            maxWidth: '350px',
          }}
        >
          <Box sx={{ margin: '12px' }}>
            <DateTimePicker
              disableFuture={disableFuture}
              label="From (UTC)"
              timezone="UTC"
              value={startTime}
              slotProps={{
                textField: {
                  error: !!startTimeErrMsg,
                  helperText: startTimeErrMsg,
                },
              }}
              onChange={(newValue) =>
                newValue &&
                setSearchParams(
                  timeRangeParamUpdater(
                    START_TIME_KEY,
                    newValue.startOf('minute'),
                  ),
                )
              }
            />
          </Box>
          <Box sx={{ margin: '12px' }}>
            <DateTimePicker
              disableFuture={disableFuture}
              label="To (UTC)"
              timezone="UTC"
              value={endTime}
              onChange={(newValue) =>
                newValue &&
                setSearchParams(
                  timeRangeParamUpdater(
                    END_TIME_KEY,
                    newValue.startOf('minute'),
                  ),
                )
              }
              slotProps={{
                textField: {
                  error: !!endTimeErrMsg,
                  helperText: endTimeErrMsg,
                },
              }}
            />
          </Box>
        </Box>
      </Popover>
    </div>
  );
}
