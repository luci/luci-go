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
import FilterListIcon from '@mui/icons-material/FilterList';
import { Box, ListItemIcon, ListItemText, MenuItem } from '@mui/material';
import { MenuItemProps } from '@mui/material/MenuItem';
import { DateTime } from 'luxon';
import { MRT_Column, MRT_RowData } from 'material-react-table';
import { forwardRef, useState } from 'react';

import { DateFilter } from '@/fleet/components/filter_dropdown/date_filter';
import { OptionsMenuOld } from '@/fleet/components/filter_dropdown/options_menu_old';
import {
  RangeFilter,
  RangeFilterValue,
} from '@/fleet/components/filter_dropdown/range_filter';
import { OptionsDropdown } from '@/fleet/components/options_dropdown';
import { DateFilterValue } from '@/fleet/types';
import { OptionValue } from '@/fleet/types/option';
import { fromLuxonDateTime, toLuxonDateTime } from '@/fleet/utils/dates';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

export type FilterOption =
  | string
  | { label: string; value: string }
  | { text: string; value: string };

export interface MRTFilterMenuItemProps<
  TData extends MRT_RowData,
  TValue = unknown,
> extends Omit<MenuItemProps, 'id'> {
  column: MRT_Column<TData, TValue>;
  closeMenu: () => void;
}

export const MRTFilterMenuItem = forwardRef<
  HTMLLIElement,
  MRTFilterMenuItemProps<MRT_RowData, unknown>
>(function MRTFilterMenuItem({ column, closeMenu, onClick, ...rest }, ref) {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);

  const filterSelectOptions = column.columnDef.filterSelectOptions as
    | FilterOption[]
    | undefined;

  const filterVariant = column.columnDef.filterVariant ?? 'multi-select';
  const isFilterable =
    filterVariant !== 'multi-select' ||
    (!!filterSelectOptions && filterSelectOptions.length > 0);

  // Local state of selected value while the dropdown is open
  const [localFilterValue, setLocalFilterValue] = useState<unknown>(undefined);

  const open = Boolean(anchorEl);

  const handleOpen = (event: React.MouseEvent<HTMLLIElement>) => {
    const currentFilterValue = column.getFilterValue();

    if (filterVariant === 'range') {
      setLocalFilterValue((currentFilterValue as RangeFilterValue) || {});
    } else if (filterVariant === 'date-range') {
      const dateOnlyRange =
        (currentFilterValue as { min?: DateOnly; max?: DateOnly }) || {};
      setLocalFilterValue({
        min: dateOnlyRange.min
          ? toLuxonDateTime(dateOnlyRange.min)?.toJSDate()
          : undefined,
        max: dateOnlyRange.max
          ? toLuxonDateTime(dateOnlyRange.max)?.toJSDate()
          : undefined,
      });
    } else {
      setLocalFilterValue(
        new Set(Array.isArray(currentFilterValue) ? currentFilterValue : []),
      );
    }
    setAnchorEl(event.currentTarget);
    onClick?.(event);
  };

  const handleClose = () => {
    setAnchorEl(null);
    closeMenu();
  };

  const handleApply = () => {
    if (filterVariant === 'range') {
      const rangeVal = (localFilterValue as RangeFilterValue) || {};
      if (rangeVal.min === undefined && rangeVal.max === undefined) {
        column.setFilterValue(undefined);
      } else {
        column.setFilterValue(rangeVal);
      }
    } else if (filterVariant === 'date-range') {
      const dateRangeVal = (localFilterValue as DateFilterValue) || {};
      if (!dateRangeVal.min && !dateRangeVal.max) {
        column.setFilterValue(undefined);
      } else {
        column.setFilterValue({
          min: dateRangeVal.min
            ? fromLuxonDateTime(DateTime.fromJSDate(dateRangeVal.min))
            : undefined,
          max: dateRangeVal.max
            ? fromLuxonDateTime(DateTime.fromJSDate(dateRangeVal.max))
            : undefined,
        });
      }
    } else {
      const selectedSet = (localFilterValue as Set<string>) || new Set();
      if (selectedSet.size === 0) {
        column.setFilterValue(undefined);
      } else {
        column.setFilterValue(Array.from(selectedSet));
      }
    }
    handleClose();
  };

  const handleReset = () => {
    if (filterVariant === 'range') {
      setLocalFilterValue({});
    } else if (filterVariant === 'date-range') {
      setLocalFilterValue({});
    } else {
      setLocalFilterValue(new Set<string>());
    }
  };

  const flipOption = (value: string) => {
    const selectedSet = (localFilterValue as Set<string>) || new Set();
    const newSelectedValues = new Set(selectedSet);
    if (newSelectedValues.has(value)) {
      newSelectedValues.delete(value);
    } else {
      newSelectedValues.add(value);
    }
    setLocalFilterValue(newSelectedValues);
  };

  const options: OptionValue[] = (filterSelectOptions || []).map((v) => {
    if (typeof v === 'string') {
      return { value: v, label: v };
    }
    // Handle { text: string, value: string } or { label: string, value: string }
    return {
      value: v.value,
      label: 'label' in v ? v.label : 'text' in v ? v.text : String(v),
    };
  });

  return (
    <MenuItem
      ref={ref}
      disabled={!isFilterable}
      {...rest}
      onClick={(e) => {
        handleOpen(e);
        if (onClick) {
          onClick(e);
        }
      }}
      onKeyDown={(e) => {
        if (e.key === 'ArrowRight' || e.key === 'Enter') {
          e.preventDefault();
          handleOpen(e as unknown as React.MouseEvent<HTMLLIElement>);
        }
        if (rest.onKeyDown) {
          rest.onKeyDown(e);
        }
      }}
    >
      <ListItemIcon>
        <FilterListIcon fontSize="small" />
      </ListItemIcon>
      <ListItemText>Filter</ListItemText>
      {open && (
        <OptionsDropdown
          open={open}
          onClick={(e) => e.stopPropagation()}
          anchorEl={anchorEl}
          onClose={handleClose}
          anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
          enableSearchInput={filterVariant === 'multi-select'}
          maxHeight={500}
          onResetClick={handleReset}
          footerButtons={['reset', 'cancel', 'apply']}
          onApply={handleApply}
          renderChild={(searchQuery, onNavigateUp) => {
            if (!isFilterable) {
              return null;
            }

            if (filterVariant === 'range') {
              return (
                <Box sx={{ p: 2 }}>
                  <RangeFilter
                    value={(localFilterValue as RangeFilterValue) || {}}
                    onChange={setLocalFilterValue}
                  />
                </Box>
              );
            }

            if (filterVariant === 'date-range') {
              return (
                <Box sx={{ p: 2 }}>
                  <DateFilter
                    value={(localFilterValue as DateFilterValue) || {}}
                    onChange={setLocalFilterValue}
                  />
                </Box>
              );
            }

            const selectedSet = (localFilterValue as Set<string>) || new Set();
            const sortedOptions = fuzzySort(searchQuery)(
              options,
              (x) => x.label,
            );
            const selectedResults = sortedOptions.filter((x) =>
              selectedSet.has(x.el.value),
            );
            const unselectedResults = sortedOptions.filter(
              (x) => !selectedSet.has(x.el.value),
            );
            const finalOptions = [...selectedResults, ...unselectedResults];

            return (
              <OptionsMenuOld
                elements={finalOptions}
                selectedElements={selectedSet}
                flipOption={flipOption}
                onNavigateUp={onNavigateUp}
              />
            );
          }}
        />
      )}
    </MenuItem>
  );
});
