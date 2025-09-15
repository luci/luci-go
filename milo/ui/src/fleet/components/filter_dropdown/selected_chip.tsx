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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import { Chip, ChipProps, ClickAwayListener } from '@mui/material';
import { ReactNode, useState } from 'react';

import { keyboardUpDownHandler } from '@/fleet/utils';

import { OptionsDropdown } from '../options_dropdown/options_dropdown';

export interface SelectedChipProps {
  dropdownContent: (searchQuery: string) => ReactNode;
  label: string;
  onApply: () => void;
}

export function SelectedChip({
  dropdownContent,
  label,
  onApply,
  onDelete,
  ...chipProps
}: ChipProps & SelectedChipProps) {
  const [anchorEl, setAnchorEL] = useState<HTMLElement | null>(null);

  return (
    <>
      <Chip
        {...chipProps}
        onClick={(event) => {
          event.stopPropagation();
          if (anchorEl) {
            setAnchorEL(null);
            return;
          }
          setAnchorEL(event.currentTarget);
        }}
        onMouseDown={(e) => {
          if (e.button === 1) {
            if (onDelete) {
              onDelete(e);
              e.stopPropagation();
            }
          }
        }}
        label={
          <p style={{ display: 'flex', alignItems: 'center' }}>
            <span
              style={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {label}
            </span>
            <ArrowDropDownIcon />
          </p>
        }
        sx={{
          maxWidth: 300,
          margin: '2px',
          ':hover': {
            cursor: 'pointer', // without it probably the textfield overrides the cursor
          },
        }}
        variant="filter"
        onDelete={onDelete}
        onKeyDown={(e) => {
          e.stopPropagation();
          keyboardUpDownHandler(e, () => {
            setAnchorEL(e.currentTarget);
            e.preventDefault();
            e.stopPropagation();
          });
          chipProps.onKeyDown?.(e);
        }}
      />
      <ClickAwayListener
        onClickAway={() => {
          setAnchorEL(null);
        }}
      >
        <OptionsDropdown
          anchorEl={anchorEl}
          open={!!anchorEl}
          anchorOrigin={{
            vertical: 'bottom',
            horizontal: 'left',
          }}
          hideBackdrop
          disableEnforceFocus
          disableRestoreFocus
          sx={{
            marginTop: '7px', // aligns similarly to the main filter bar dropdown. Would be nice to make it more robust in the future
            pointerEvents: 'none', // prevents the portal from capturing clicks and allows to click on the search bar
          }}
          slotProps={{
            list: {
              sx: {
                pointerEvents: 'auto', // restores the click
              },
            },
          }}
          enableSearchInput
          renderChild={dropdownContent}
          onApply={() => {
            anchorEl?.focus();
            setAnchorEL(null);
            onApply();
          }}
          onClose={() => {
            anchorEl?.focus();
            setAnchorEL(null);
          }}
        />
      </ClickAwayListener>
    </>
  );
}
