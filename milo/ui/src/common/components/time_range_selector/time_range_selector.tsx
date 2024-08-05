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
import React, { useState } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  getSelectedOption,
  getStartEndTime,
  CUSTOMIZE_OPTION,
  MenuOption,
  RELATIVE_TIME_OPTIONS,
  CUSTOMIZE_OPTION_DISPLAY_TEXT,
  updateSeletorInSearchParameter,
  END_TIME_KEY,
  START_TIME_KEY,
  OPTION_KEY,
} from './time_range_selector_utils';

/**
 * TimeRangeSelector implement a component that supports absolute and relative time range selection.
 * It saves the selected time range option to search params. It also read the search params to set existing selection.
 * Parent can use getAbsoluteStartEndTime function to obtain the selected start and end time.
 */
export function TimeRangeSelector() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [pickerAnchorEl, setPickerAnchorElPicker] =
    useState<null | HTMLElement>(null);
  const selectedOption = getSelectedOption(searchParams);
  const { startTime, endTime } = getStartEndTime(searchParams);

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setMenuAnchorEl(event.currentTarget);
  };

  const handleMenuItemClick = (
    option: MenuOption,
    event: React.MouseEvent<HTMLLIElement>,
  ) => {
    // Update search params
    setSearchParams(
      updateSeletorInSearchParameter(OPTION_KEY, option.toString()),
    );
    if (option === CUSTOMIZE_OPTION) {
      setPickerAnchorElPicker(event.currentTarget);
      return;
    }
    setSearchParams(updateSeletorInSearchParameter(START_TIME_KEY, null));
    setSearchParams(updateSeletorInSearchParameter(END_TIME_KEY, null));
    // Close the main menu
    setMenuAnchorEl(null);
  };

  const handlePopoverClose = () => {
    setPickerAnchorElPicker(null);
    setMenuAnchorEl(null);
  };

  return (
    <div>
      <Button
        variant="outlined"
        onClick={handleClick}
        sx={{ display: 'flex', gap: '7px' }}
        data-testid="time-button"
      >
        <AccessTime />
        {selectedOption === CUSTOMIZE_OPTION ? (
          <>
            {startTime && startTime.toFormat('yyyy-MM-dd HH:mm')} -{' '}
            {endTime && endTime.toFormat('yyyy-MM-dd HH:mm')}
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
          }}
        >
          <Box sx={{ margin: '12px' }}>
            <DateTimePicker
              label="From (UTC)"
              timezone="UTC"
              value={startTime}
              onChange={(newValue) =>
                newValue &&
                setSearchParams(
                  updateSeletorInSearchParameter(
                    START_TIME_KEY,
                    newValue.startOf('minute').toUnixInteger().toString(),
                  ),
                )
              }
            />
          </Box>
          <Box sx={{ margin: '12px' }}>
            <DateTimePicker
              label="To (UTC)"
              timezone="UTC"
              value={endTime}
              onChange={(newValue) =>
                newValue &&
                setSearchParams(
                  updateSeletorInSearchParameter(
                    END_TIME_KEY,
                    newValue.startOf('minute').toUnixInteger().toString(),
                  ),
                )
              }
            />
          </Box>
        </Box>
      </Popover>
    </div>
  );
}
