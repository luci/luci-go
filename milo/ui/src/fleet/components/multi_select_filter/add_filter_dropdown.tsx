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

import SearchIcon from '@mui/icons-material/Search';
import { Divider, MenuItem, TextField, Menu } from '@mui/material';
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
  const [anchorEl2, setAnchorEL2] = useState<HTMLElement | null>(null);
  const [open2, setOpen2] = useState<number>();

  const [searchQuery, setSearchQuery] = useState('');

  const closeMenu = () => {
    setOpen2(undefined);
    setAnchorEL2(null);
    setAnchorEL(null);
  };

  const closeOnEscape = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      if (searchQuery === '') {
        closeMenu();
      }
      setSearchQuery('');
      e.currentTarget.blur();
    }
    e.stopPropagation();
  };

  return (
    <Menu open={!!anchorEl} anchorEl={anchorEl} onClose={closeMenu}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 15,
          padding: '0 10px',
        }}
      >
        <SearchIcon />
        <TextField
          placeholder="Search"
          variant="standard"
          onChange={(e) => setSearchQuery(e.currentTarget.value)}
          value={searchQuery}
          onKeyDown={closeOnEscape}
          // TODO(pietroscutta) investigate accessibility implications of autofocus
          // eslint-disable-next-line jsx-a11y/no-autofocus
          autoFocus
        />
      </div>
      <Divider />
      {filterOptions
        .filter(
          (opt) =>
            searchQuery === '' ||
            opt.label.toLowerCase().startsWith(searchQuery.toLowerCase()),
        ) // TODO(pietroscutta) would be nice if this was fuzzy
        .map((o, idx) => (
          <MenuItem
            onClick={(event) => {
              // The onClick fires also when closing the menu
              if (open2 === undefined) {
                setOpen2(idx);
                setAnchorEL2(event.currentTarget);
              }
            }}
            key={`item-${idx}`}
            disableRipple
          >
            <button
              // Is there a better way to have semantic html without
              // the styling / functionality ?
              style={{
                all: 'initial',
              }}
            >
              <p>{o.label}</p>
            </button>
            <OptionsDropdown
              onClose={(_, reason) => {
                setOpen2(undefined);
                setAnchorEL2(null);
                // TODO(pietroscutta) it would be cool if the outer menu didnt close if the
                // click is inside it
                if (reason === 'backdropClick') {
                  setAnchorEL(null);
                }
              }}
              setSelectedOptions={setSelectedOptions}
              selectedOptions={selectedOptions}
              anchorEl={anchorEl2}
              open={anchorEl2 !== null && open2 === idx}
              option={o}
            />
          </MenuItem>
        ))}
    </Menu>
  );
}
