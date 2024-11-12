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
import { useState } from 'react';

import { OptionsDropdown } from './options_dropdown';
import { FilterOption, SelectedFilters } from './types';

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
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 15,
          padding: '0 10px',
        }}
      >
        <TextField
          placeholder="search"
          variant="standard"
          onChange={(e) => setSearchQuery(e.currentTarget.value)}
          value={searchQuery}
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
          InputProps={{
            startAdornment: <SearchIcon />,
            onKeyDown: (e) => {
              if (e.key === 'Escape') {
                e.currentTarget.blur();
                e.stopPropagation();
              }
            },
          }}
        />
      </div>
      {filterOptions
        .filter(
          (option) =>
            searchQuery === '' ||
            option.label.toLowerCase().startsWith(searchQuery.toLowerCase()),
        ) // TODO(pietroscutta) would be nice if this was fuzzy
        .map((option, idx) => (
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
          >
            <button
              // Is there a better way to have semantic html without
              // the styling / functionality ?
              css={{
                all: 'initial',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                width: '100%',
              }}
            >
              <Typography variant="body2">{option.label}</Typography>
              <ArrowRightIcon />
            </button>
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
