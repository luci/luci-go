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
import { Chip, ChipProps } from '@mui/material';
import { JSX } from 'react';

export interface BaseFilterChipProps
  extends Omit<
    ChipProps,
    'label' | 'color' | 'variant' | 'icon' | 'onDelete' | 'size'
  > {
  label: string;
  active: boolean;
  onClear?: () => void;
}

/**
 * A base component for creating filter chips.
 * It handles the common logic for displaying an active (primary, filled)
 * or inactive (default, outlined) state, and ensures consistent styling
 * for hover and focus states.
 */
export function BaseFilterChip({
  label,
  active,
  onClear,
  ...rest
}: BaseFilterChipProps) {
  const chipIcon: JSX.Element | undefined = active ? undefined : (
    <AddIcon fontSize="small" />
  );
  const chipVariant: 'filled' | 'outlined' = active ? 'filled' : 'outlined';
  const chipColor: 'default' | 'primary' = active ? 'primary' : 'default';

  return (
    <Chip
      {...rest}
      label={label}
      icon={chipIcon}
      variant={chipVariant}
      color={chipColor}
      onDelete={
        onClear
          ? (e) => {
              e.preventDefault();
              onClear();
            }
          : undefined
      }
      size="small"
      sx={{
        // When a primary chip is hovered or focused, its background becomes darker.
        // Ensure the text remains readable by setting it to white.
        '&.MuiChip-colorPrimary:hover, &.MuiChip-colorPrimary.Mui-focusVisible':
          {
            color: 'white',
          },
        ...rest.sx,
      }}
    />
  );
}
