// Copyright 2025 The LUCI Authors.
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

import AddIcon from '@mui/icons-material/Add';
import { Checkbox, Chip, ListItemText, Menu, MenuItem } from '@mui/material';
import React, { JSX, useState } from 'react';

export interface CategoryOption {
  value: string; // The internal value/key for the option
  label: string; // The display label for the option
}

export interface MultiSelectCategoryChipProps {
  categoryName: string;
  availableOptions: readonly CategoryOption[];
  selectedItems: Set<string>; // Set of selected option 'value's
  onSelectedItemsChange: (newSelection: Set<string>) => void;
}

/**
 * A dynamic chip for filtering by a single category with a fixed set of options.
 * Manages its appearance and the selection menu based on selected items.
 */
export function MultiSelectCategoryChip({
  categoryName,
  availableOptions,
  selectedItems,
  onSelectedItemsChange,
}: MultiSelectCategoryChipProps) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);

  const handleClickChip = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleCloseMenu = () => {
    setAnchorEl(null);
  };

  const handleToggleItemInMenu = (itemValue: string) => {
    const newSelection = new Set(selectedItems);
    if (newSelection.has(itemValue)) {
      newSelection.delete(itemValue);
    } else {
      newSelection.add(itemValue);
    }
    onSelectedItemsChange(newSelection);
  };

  const handleClearFilter = () => {
    onSelectedItemsChange(new Set());
    handleCloseMenu();
  };

  let chipLabel: string;
  let chipIcon: JSX.Element | undefined = undefined;
  let chipVariant: 'filled' | 'outlined' = 'outlined';
  let chipColor: 'default' | 'primary' = 'default';
  let chipOnDelete: (() => void) | undefined = undefined;

  if (selectedItems.size === 0) {
    chipLabel = `${categoryName}`;
    chipIcon = <AddIcon fontSize="small" />;
    chipVariant = 'outlined';
    chipColor = 'default';
  } else if (selectedItems.size === 1) {
    const singleItemValue = textForOption(availableOptions, selectedItems, 0);
    chipLabel = `${categoryName}: ${singleItemValue}`;
    chipVariant = 'filled';
    chipColor = 'primary';
    chipOnDelete = handleClearFilter;
  } else {
    const firstValue = textForOption(availableOptions, selectedItems, 0);
    const secondValue = textForOption(availableOptions, selectedItems, 1);
    chipLabel = `${selectedItems.size} | ${categoryName}: ${firstValue}, ${secondValue}${selectedItems.size > 2 ? ', ...' : ''}`;
    chipVariant = 'filled';
    chipColor = 'primary';
    chipOnDelete = handleClearFilter;
  }

  return (
    <>
      <Chip
        label={chipLabel}
        icon={chipIcon}
        variant={chipVariant}
        color={chipColor}
        onClick={handleClickChip}
        onDelete={chipOnDelete}
        size="small"
      />
      <Menu
        id={`${categoryName.toLowerCase().replace(/\s+/g, '-')}-filter-menu`}
        anchorEl={anchorEl}
        open={open}
        onClose={handleCloseMenu}
        MenuListProps={{
          'aria-labelledby': `${categoryName
            .toLowerCase()
            .replace(/\s+/g, '-')}-filter-chip`,
        }}
      >
        {availableOptions.map((option) => (
          <MenuItem
            key={option.value}
            onClick={() => handleToggleItemInMenu(option.value)}
          >
            <Checkbox checked={selectedItems.has(option.value)} size="small" />
            <ListItemText primary={option.label} />
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}

function textForOption(
  availableOptions: readonly CategoryOption[],
  selectedItems: Set<string>,
  index: number,
): string {
  if (index >= selectedItems.size) {
    return '';
  }
  const singleItemValue = Array.from(selectedItems)[index];
  return (
    availableOptions.find((opt) => opt.value === singleItemValue)?.label ||
    singleItemValue
  );
}
