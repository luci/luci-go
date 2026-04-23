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
import {
  Box,
  ListItemIcon,
  ListItemText,
  MenuItem,
  TextField,
} from '@mui/material';
import { MenuItemProps } from '@mui/material/MenuItem';
import { useForkRef } from '@mui/material/utils';
import { DateTime } from 'luxon';
import { MRT_Column, MRT_RowData } from 'material-react-table';
import { forwardRef, useRef, useState } from 'react';

import { MRT_INTERNAL_COLUMNS } from '@/fleet/components/columns/use_mrt_column_management';
import { FC_ColumnDef } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { DateFilter } from '@/fleet/components/filter_dropdown/date_filter';
import { MenuSkeleton } from '@/fleet/components/filter_dropdown/menu_skeleton';
import { OptionsMenuOld } from '@/fleet/components/filter_dropdown/options_menu_old';
import {
  RangeFilter,
  RangeFilterValue,
} from '@/fleet/components/filter_dropdown/range_filter';
import { OptionsDropdown } from '@/fleet/components/options_dropdown';
import { DateFilterValue } from '@/fleet/types';
import { OptionValue } from '@/fleet/types/option';
import { fromLuxonDateTime, toLuxonDateTime } from '@/fleet/utils/dates';
import { stripQuotes } from '@/fleet/utils/filters';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import {
  parseCommaSeparatedText,
  formatCommaSeparatedText,
} from './mrt_filter_menu_item_utils';

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
  const isInternalColumn = MRT_INTERNAL_COLUMNS.has(column.id);

  const isFilterable =
    !isInternalColumn &&
    column.columnDef.enableColumnFilter !== false &&
    (filterVariant !== 'multi-select' ||
      (!!filterSelectOptions && filterSelectOptions.length > 0));

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

  // Local state of selected value while the dropdown is open
  const [localFilterValue, setLocalFilterValue] = useState<unknown>(undefined);

  const innerRef = useRef<HTMLLIElement>(null);
  const handleForkRef = useForkRef(ref, innerRef);

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
    } else if (filterVariant === 'text') {
      if (currentFilterValue) {
        const valSet = Array.isArray(currentFilterValue)
          ? new Set(currentFilterValue.map(String))
          : new Set(parseCommaSeparatedText(String(currentFilterValue)));
        setLocalFilterValue(valSet);
      } else {
        setLocalFilterValue(new Set());
      }
    } else {
      const valArray = Array.isArray(currentFilterValue)
        ? currentFilterValue
        : typeof currentFilterValue === 'string'
          ? parseCommaSeparatedText(currentFilterValue)
          : [];

      const set = new Set<string>();
      valArray.forEach((v) => {
        const s = String(v);
        const unquoted = stripQuotes(s);
        const quoted = `"${unquoted}"`;

        const match = options.find(
          (opt) =>
            opt.value === s || opt.value === unquoted || opt.value === quoted,
        );
        if (match) {
          set.add(match.value);
        } else {
          set.add(s);
        }
      });
      setLocalFilterValue(set);
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
    } else if (filterVariant === 'text') {
      setLocalFilterValue(new Set<string>());
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

  return (
    <MenuItem
      ref={handleForkRef}
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
          anchorEl={innerRef.current}
          onClose={handleClose}
          anchorOrigin={{ vertical: 'top', horizontal: 'right' }}
          transformOrigin={{ vertical: 'top', horizontal: 'left' }}
          enableSearchInput={filterVariant === 'multi-select'}
          maxHeight={500}
          onResetClick={handleReset}
          footerButtons={['reset', 'cancel', 'apply']}
          onApply={handleApply}
          closeOnScroll={true}
          disableScrollUpdate={true}
          renderChild={(searchQuery, onNavigateUp) => {
            if (!isFilterable) {
              return null;
            }

            const isLoadingOptions = (
              column.columnDef.meta as
                | { isLoadingOptions?: boolean }
                | undefined
            )?.isLoadingOptions;
            if (isLoadingOptions) {
              return <MenuSkeleton itemCount={5} maxHeight={400} />;
            }

            if (filterVariant === 'range') {
              const def = column.columnDef as FC_ColumnDef<MRT_RowData>;
              return (
                <Box sx={{ p: 2 }}>
                  <RangeFilter
                    value={(localFilterValue as RangeFilterValue) || {}}
                    onChange={setLocalFilterValue}
                    min={def.filterRangeMin}
                    max={def.filterRangeMax}
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

            if (filterVariant === 'text') {
              const currentVal = formatCommaSeparatedText(
                (localFilterValue as Set<string>) || new Set(),
              );

              return (
                <Box
                  sx={{
                    p: 2,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1,
                  }}
                >
                  <TextField
                    variant="outlined"
                    size="small"
                    value={currentVal || ''}
                    placeholder="Filter value..."
                    inputProps={{
                      'aria-label': 'Filter value',
                    }}
                    sx={{ width: '100%' }}
                    onChange={(e) => {
                      const val = e.target.value;
                      setLocalFilterValue(
                        val ? new Set(parseCommaSeparatedText(val)) : new Set(),
                      );
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.stopPropagation();
                        e.preventDefault();
                        handleApply();
                      }
                    }}
                  />
                  <Box sx={{ fontSize: '0.75rem', color: 'text.secondary' }}>
                    Type a value to filter. Multiple values can be
                    comma-separated.
                  </Box>
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
