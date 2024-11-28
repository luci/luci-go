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

import ArrowRightIcon from '@mui/icons-material/ArrowRight';
import SearchIcon from '@mui/icons-material/Search';
import { MenuItem, TextField, Menu, Typography } from '@mui/material';
import _ from 'lodash';
import { useRef, useState } from 'react';

import { OptionsDropdown } from './options_dropdown';
import { FilterOption, SelectedFilters } from './types';
import { fuzzySort, hasAnyModifier, keyboardUpDownHandler } from './utils';

export function AddFilterDropdown({
  filterOptions,
  selectedOptions,
  setSelectedOptions,
  anchorEl,
  setAnchorEL,
}: {
  filterOptions: FilterOption[];
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
}) {
  const [anchorElInner, setAnchorELInner] = useState<HTMLElement | null>(null);
  const [open, setOpen] = useState<number>();
  const searchInput = useRef<HTMLInputElement>(null);

  const [searchQuery, setSearchQuery] = useState('');

  const closeMenu = () => {
    setOpen(undefined);
    setAnchorELInner(null);
    setAnchorEL(null);
  };

  return (
    <Menu
      open={!!anchorEl}
      anchorEl={anchorEl}
      onClose={closeMenu}
      elevation={2}
      MenuListProps={{
        sx: {
          maxHeight: 300,
        },
      }}
      onKeyDown={(e) => {
        keyboardUpDownHandler(e);
        switch (e.key) {
          case 'Delete':
          case 'Cancel':
          case 'Backspace':
            setSearchQuery('');
            searchInput.current?.focus();
        }
        // if the key is a single alphanumeric character without modifier
        if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
          searchInput.current?.focus();
          setSearchQuery((old) => old + e.key);
          e.preventDefault(); // Avoid race condition to type twice in the input
        }
      }}
    >
      <div
        role="menu"
        tabIndex={0}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 15,
          padding: '0 10px',
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            searchInput.current?.focus();
          }
        }}
      >
        <TextField
          inputRef={searchInput}
          placeholder="search"
          variant="standard"
          onChange={(e) => {
            setSearchQuery(e.currentTarget.value);
          }}
          value={searchQuery}
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          onKeyDown={(e) => {
            if (e.key === 'Escape' || (e.key === 'j' && e.ctrlKey)) {
              e.currentTarget.parentElement?.focus();
              e.stopPropagation();
            }
            if (
              e.key === 'Backspace' ||
              e.key === 'Delete' ||
              e.key === 'Cancel'
            ) {
              e.stopPropagation();
            }
          }}
          slotProps={{
            input: {
              startAdornment: <SearchIcon />,
            },
          }}
        />
      </div>
      {_.uniqBy(
        // It's possible to have duplicate results if the searchQuery matches
        // multiple second level options of the same first level option
        //
        // I.E. {Status: [active, inactive]} with searchQuery='active'
        fuzzySort(searchQuery)(filterOptions, (o) => [
          o.label,
          ...o.options.map((o) => o.label),
        ]),
        (o) => o.value,
      ).map((option, idx) => (
        <MenuItem
          onClick={(event) => {
            // The onClick fires also when closing the menu
            if (open === undefined) {
              setOpen(idx);
              setAnchorELInner(event.currentTarget);
            }
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowRight') {
              e.currentTarget.click();
            }
          }}
          key={`item-${option.value}-${idx}`}
          disableRipple
          selected={open === idx}
          sx={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            width: '100%',
          }}
        >
          <Typography variant="body2">{option.label}</Typography>
          <ArrowRightIcon />
          <OptionsDropdown
            onKeyDown={(e) => {
              if (e.key === 'ArrowLeft') {
                setOpen(undefined);
                setAnchorELInner(null);
              }
            }}
            onClose={(_, reason) => {
              setOpen(undefined);
              setAnchorELInner(null);
              // TODO(pietroscutta) it would be cool if the outer menu didnt close if the
              // click is inside it
              if (reason === 'backdropClick') {
                setAnchorEL(null);
              }
            }}
            setSelectedOptions={setSelectedOptions}
            selectedOptions={selectedOptions}
            anchorEl={anchorElInner}
            open={anchorElInner !== null && open === idx}
            option={option}
          />
        </MenuItem>
      ))}
    </Menu>
  );
}
