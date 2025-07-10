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

import { Box, Button, Popover, TextField, Typography } from '@mui/material';
import React, { useState, useEffect } from 'react';

import { BaseFilterChip } from './base_filter_chip';

export interface TextInputFilterChipProps {
  categoryName: string;
  value: string | null;
  onValueChange: (newValue: string | null) => void;
}

/**
 * A chip for filtering by a category that accepts free-text input.
 * It displays as a placeholder chip when no value is set, and as an
 * active filter chip when a value is provided. Clicking the chip
 * opens a popover with a text field to enter or edit the filter value.
 */
export function TextInputFilterChip({
  categoryName,
  value,
  onValueChange,
}: TextInputFilterChipProps) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [inputValue, setInputValue] = useState(value || '');
  const open = Boolean(anchorEl);

  useEffect(() => {
    setInputValue(value || '');
  }, [value]);

  const handleClickChip = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
    // Reset input to current filter value when closing without applying
    setInputValue(value || '');
  };

  const handleApply = () => {
    onValueChange(inputValue.trim() === '' ? null : inputValue.trim());
    handleClose();
  };

  const handleClear = () => {
    onValueChange(null);
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      handleApply();
    } else if (event.key === 'Escape') {
      handleClose();
    }
  };

  let chipLabel: string;

  if (!value) {
    chipLabel = categoryName;
  } else {
    chipLabel = `${categoryName}: ${value}`;
  }

  return (
    <>
      <BaseFilterChip
        label={chipLabel}
        active={!!value}
        onClick={handleClickChip}
        onClear={value ? handleClear : undefined}
      />
      <Popover
        id={`${categoryName.toLowerCase().replace(/\s+/g, '-')}-filter-popover`}
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
      >
        <Box sx={{ p: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <Typography variant="body2">{`Filter by ${categoryName}`}</Typography>
          <TextField
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            label="Value"
            variant="outlined"
            size="small"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
          />
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1 }}>
            <Button onClick={handleClose} size="small">
              Cancel
            </Button>
            <Button onClick={handleApply} variant="contained" size="small">
              Apply
            </Button>
          </Box>
        </Box>
      </Popover>
    </>
  );
}
