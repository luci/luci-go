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
import { MenuItem, Menu } from '@mui/material';
import _ from 'lodash';
import { useMemo, useRef, useState } from 'react';

import { Option, SelectedOptions } from '@/fleet/types';

import { fuzzySort, hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { HighlightCharacter } from '../highlight_character';
import { OptionsDropdown } from '../options_dropdown';
import { SearchInput } from '../search_input';

export function AddFilterDropdown({
  filterOptions,
  selectedOptions,
  setSelectedOptions,
  anchorEl,
  setAnchorEL,
}: {
  filterOptions: Option[];
  selectedOptions: SelectedOptions;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedOptions>>;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
}) {
  const [anchorElInner, setAnchorELInner] = useState<HTMLElement | null>(null);
  const [open, setOpen] = useState<number>();
  const searchInput = useRef<HTMLInputElement>(null);
  const menuListRef = useRef<HTMLUListElement>(null);

  const [searchQuery, setSearchQuery] = useState('');

  const closeInnerMenu = () => {
    setOpen(undefined);
    setAnchorELInner(null);
  };
  const closeMenu = () => {
    closeInnerMenu();
    setAnchorEL(null);
  };

  const filterResults = useMemo(
    () =>
      Object.values(
        _.groupBy(
          fuzzySort(searchQuery)(
            filterOptions,
            (el) => el.options,
            (el) => el.label,
          ),
          (o) => o.parent?.value ?? o.el.value,
        ),
      ),
    [filterOptions, searchQuery],
  );

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
        ref: menuListRef,
      }}
      onKeyDown={(e) => {
        keyboardUpDownHandler(e);
        switch (e.key) {
          case 'Delete':
          case 'Cancel':
          case 'Backspace':
            setSearchQuery('');
            searchInput.current?.focus();
            closeInnerMenu();
        }
        // if the key is a single alphanumeric character without modifier
        if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
          closeInnerMenu();
          searchInput.current?.focus();
          setSearchQuery((old) => old + e.key);
          e.preventDefault(); // Avoid race condition to type twice in the input
        }
      }}
    >
      <SearchInput
        searchInput={searchInput}
        searchQuery={searchQuery}
        onChange={(e) => {
          setSearchQuery(e.currentTarget.value);
        }}
      />
      {filterResults.map((searchResult, idx) => {
        const parent = searchResult[0].parent ?? searchResult[0].el;
        const parentMatches = searchResult
          .filter((sr) => sr.parent === undefined)
          .at(0)?.matches;
        const childrenMatches = Object.fromEntries(
          searchResult
            .filter((sr) => sr.parent !== undefined)
            .map((sr) => [sr.el.value, sr.matches]),
        );

        return (
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
            key={`item-${parent.value}-${idx}`}
            disableRipple
            selected={open === idx}
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              width: '100%',
              minHeight: 'auto',
            }}
          >
            <HighlightCharacter variant="body2" highlights={parentMatches}>
              {parent.label}
            </HighlightCharacter>
            <ArrowRightIcon />
            <OptionsDropdown
              onKeyDown={(e) => {
                if (e.key === 'ArrowLeft') {
                  closeInnerMenu();
                }
              }}
              onClose={(rawE, reason) => {
                closeInnerMenu();

                const e = rawE as MouseEvent;
                const menuRect = menuListRef.current?.getBoundingClientRect();

                const isInsideMenu =
                  menuRect &&
                  e.pageX > menuRect.left &&
                  e.pageX < menuRect.right &&
                  e.pageY > menuRect.top &&
                  e.pageY < menuRect.bottom;
                if (reason === 'backdropClick' && !isInsideMenu) {
                  setAnchorEL(null);
                }
              }}
              setSelectedOptions={setSelectedOptions}
              selectedOptions={selectedOptions}
              anchorEl={anchorElInner}
              open={anchorElInner !== null && open === idx}
              option={parent}
              highlightedCharacters={childrenMatches}
              transformOrigin={{
                vertical: 'center',
                horizontal: 'left',
              }}
              sx={{
                transform: 'translate(0, 25px)',
              }}
            />
          </MenuItem>
        );
      })}
    </Menu>
  );
}
