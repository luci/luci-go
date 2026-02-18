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
import { ListItemIcon, ListItemText, MenuItem } from '@mui/material';
import { MenuItemProps } from '@mui/material/MenuItem';
import { MRT_Column, MRT_RowData } from 'material-react-table';
import { forwardRef, useState } from 'react';

import { OptionsMenuOld } from '@/fleet/components/filter_dropdown/options_menu_old';
import { OptionsDropdown } from '@/fleet/components/options_dropdown';
import { OptionValue } from '@/fleet/types/option';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

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
  const isFilterable = !!filterSelectOptions && filterSelectOptions.length > 0;

  // Local state of selected values while the dropdown is open
  const defaultSelected = (column.getFilterValue() as string[]) || [];
  const [selectedValues, setSelectedValues] = useState<Set<string>>(
    new Set(defaultSelected),
  );

  const open = Boolean(anchorEl);

  const handleOpen = (event: React.MouseEvent<HTMLLIElement>) => {
    setSelectedValues(new Set((column.getFilterValue() as string[]) || []));
    setAnchorEl(event.currentTarget);
    onClick?.(event);
  };

  const handleClose = () => {
    setAnchorEl(null);
    closeMenu();
  };

  const handleApply = () => {
    if (selectedValues.size === 0) {
      column.setFilterValue(undefined);
    } else {
      column.setFilterValue(Array.from(selectedValues));
    }
    handleClose();
  };

  const handleReset = () => {
    setSelectedValues(new Set());
  };

  const flipOption = (value: string) => {
    const newSelectedValues = new Set(selectedValues);
    if (newSelectedValues.has(value)) {
      newSelectedValues.delete(value);
    } else {
      newSelectedValues.add(value);
    }
    setSelectedValues(newSelectedValues);
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
          enableSearchInput={true}
          maxHeight={500}
          onResetClick={handleReset}
          footerButtons={['reset', 'cancel', 'apply']}
          onApply={handleApply}
          renderChild={(searchQuery, onNavigateUp) => {
            if (!isFilterable) {
              return null;
            }
            const sortedOptions = fuzzySort(searchQuery)(
              options,
              (x) => x.label,
            );
            const selectedResults = sortedOptions.filter((x) =>
              selectedValues.has(x.el.value),
            );
            const unselectedResults = sortedOptions.filter(
              (x) => !selectedValues.has(x.el.value),
            );
            const finalOptions = [...selectedResults, ...unselectedResults];

            return (
              <OptionsMenuOld
                elements={finalOptions}
                selectedElements={selectedValues}
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
