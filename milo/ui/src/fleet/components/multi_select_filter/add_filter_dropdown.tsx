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
import { Backdrop, Card, MenuItem, MenuList } from '@mui/material';
import _ from 'lodash';
import { useEffect, useMemo, useRef, useState } from 'react';

import { SelectedOptions } from '@/fleet/types';
import { OptionCategory, OptionValue } from '@/fleet/types/option';
import { fuzzySort, SortedElement } from '@/fleet/utils/fuzzy_sort';

import { hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { HighlightCharacter } from '../highlight_character';
import { Footer } from '../options_dropdown/footer';
import { SearchInput } from '../search_input';

import { OptionsMenu } from './options_menu';

export function AddFilterDropdown({
  filterOptions,
  selectedOptions: initSelectedOptions,
  setSelectedOptions,
  anchorEl,
  setAnchorEL,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedOptions>>;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
}) {
  const [anchorElInner, setAnchorELInner] = useState<HTMLElement | null>(null);
  const [openCategory, setOpenCategory] = useState<string | undefined>(
    undefined,
  );
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (anchorEl) {
      searchInput.current?.focus();
    }
  }, [anchorEl]);

  const [searchQuery, setSearchQuery] = useState('');

  const cardRef = useRef<HTMLDivElement>(null);
  const innerCardRef = useRef<HTMLDivElement>(null);

  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(initSelectedOptions);

  const flipOption = (parentValue: string, o2Value: string) => {
    const currentValues = tempSelectedOptions[parentValue] ?? [];

    const newValues = currentValues.includes(o2Value)
      ? currentValues.filter((v) => v !== o2Value)
      : currentValues.concat(o2Value);

    setTempSelectedOptions({
      ...(tempSelectedOptions ?? {}),
      [parentValue]: newValues,
    });
  };

  useEffect(() => {
    setTempSelectedOptions(initSelectedOptions);
  }, [initSelectedOptions, openCategory]);

  const anchorRect = anchorEl?.getBoundingClientRect();
  // need to get a parent for an anchor to calculate baseline as hamburger menu may be open
  const anchorParentRect = anchorEl?.parentElement?.getBoundingClientRect();

  if (cardRef.current) {
    if (anchorRect) {
      cardRef.current.style.left = `${anchorRect.left - (anchorParentRect?.left || 0)}px`;
      cardRef.current.style.top = `${anchorEl?.parentElement?.getBoundingClientRect().height || anchorRect.height}px`;
    }
  }

  if (innerCardRef.current) {
    const outerMenuRect = anchorElInner?.getBoundingClientRect();
    if (outerMenuRect && anchorRect) {
      innerCardRef.current.style.left = `${anchorRect.left - (anchorParentRect?.left || 0) + outerMenuRect.width + 2}px`;
      if (outerMenuRect.top > window.innerHeight / 2) {
        innerCardRef.current.style.top = '';
        innerCardRef.current.style.bottom = `${-outerMenuRect.bottom + anchorRect.top - 10}px`;
      } else {
        innerCardRef.current.style.bottom = '';
        innerCardRef.current.style.top = `${outerMenuRect.top - anchorRect.top - 10}px`;
      }
    }
  }

  const closeInnerMenu = () => {
    setOpenCategory(undefined);
    anchorElInner?.focus();
    setAnchorELInner(null);
  };
  const closeMenu = () => {
    setSearchQuery('');
    closeInnerMenu();
    setAnchorEL(null);
  };

  const filterResults = useMemo(() => {
    const flatMapWithParent = filterOptions.flatMap((category) => [
      { ...category, parent: undefined },
      ...category.options.map((o) => ({ ...o, parent: category })),
    ]);
    const groupsByCategory = Object.values(
      _.groupBy(
        fuzzySort(searchQuery)(flatMapWithParent, (el) => el.label),
        (o) => o.el.parent?.value ?? o.el.value,
      ),
    );
    const categories = groupsByCategory.map((g) => ({
      parent: g.find(
        (x) => x.el.parent === undefined,
      ) as SortedElement<OptionCategory>,
      children: g
        .filter((x) => x.el.parent !== undefined)
        .map((c) => c as SortedElement<OptionValue>),
    }));
    if (searchQuery === '') return categories;
    return categories.filter(
      (g) =>
        g.parent.score > 0 ||
        (g.children.length > 0 && g.children[0].score > 0),
    );
  }, [filterOptions, searchQuery]);

  const applyOptions = () => {
    closeMenu();
    setSelectedOptions(tempSelectedOptions);
  };

  const handleRandomTextInput: (
    e: React.KeyboardEvent<HTMLUListElement>,
  ) => void = (e: React.KeyboardEvent<HTMLUListElement>) => {
    // allow user to search when any alphanumeric key has been pressed
    if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
      closeInnerMenu();
      searchInput.current?.focus();
      setSearchQuery((old) => old + e.key);
      e.preventDefault(); // Avoid race condition to type twice in the input
    }
  };

  return (
    <>
      <Backdrop
        open={!!anchorEl}
        onClick={() => {
          closeMenu();
        }}
        invisible={true}
        sx={{
          zIndex: 2,
        }}
      ></Backdrop>
      <div
        css={{
          zIndex: 3,
          position: 'absolute',
          display: anchorEl === null ? 'none' : 'block',
        }}
      >
        <Card
          elevation={2}
          onClick={(e) => e.stopPropagation()}
          sx={{ position: 'absolute' }}
          ref={cardRef}
        >
          <MenuList
            sx={{
              minWidth: 200,
              maxHeight: 300,
              overflow: 'auto',
            }}
            onKeyDown={(e) => {
              keyboardUpDownHandler(e);
              switch (e.key) {
                case 'Delete':
                case 'Cancel':
                case 'Backspace':
                  setSearchQuery('');
                  searchInput.current?.focus();
                  break;
                case 'Escape':
                  closeMenu();
              }

              handleRandomTextInput(e);
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
              const parent = searchResult.parent;
              const parentMatches = parent.matches;

              return (
                <MenuItem
                  onClick={(event) => {
                    // The onClick fires also when closing the menu
                    if (openCategory !== parent.el.value) {
                      setOpenCategory(parent.el.value);
                      setAnchorELInner(event.currentTarget);
                    } else {
                      setOpenCategory(undefined);
                      setAnchorELInner(null);
                    }
                  }}
                  onKeyDown={(e) => {
                    if (e.key === 'ArrowRight') {
                      e.currentTarget.click();
                    }
                  }}
                  key={`item-${parent.el.value}-${idx}`}
                  disableRipple
                  selected={openCategory === parent.el.value}
                  sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    width: '100%',
                    minHeight: 'auto',
                  }}
                >
                  <HighlightCharacter
                    variant="body2"
                    highlightIndexes={parentMatches}
                  >
                    {parent.el.label}
                  </HighlightCharacter>
                  <ArrowRightIcon />
                </MenuItem>
              );
            })}
          </MenuList>
        </Card>
        <div ref={innerCardRef} css={{ position: 'absolute' }}>
          {anchorElInner &&
            innerCardRef.current &&
            openCategory !== undefined && (
              <Card onClick={(e) => e.stopPropagation()}>
                <MenuList
                  variant="selectedMenu"
                  sx={{
                    maxHeight: 400,
                    width: 300,
                  }}
                  onKeyDown={(e: React.KeyboardEvent<HTMLUListElement>) => {
                    if (e.key === 'Tab') {
                      closeMenu();
                    }
                    if (e.key === 'Escape' || e.key === 'ArrowLeft') {
                      closeInnerMenu();
                    }
                    if (e.key === 'Enter' && e.ctrlKey) {
                      applyOptions();
                    }
                    if (
                      e.key === 'Delete' ||
                      e.key === 'Backspace' ||
                      e.key === 'Cancel'
                    ) {
                      setSearchQuery('');
                      searchInput.current?.focus();
                    }

                    handleRandomTextInput(e);
                  }}
                >
                  <OptionsMenu
                    elements={
                      filterResults.find(
                        (r) => r.parent.el.value === openCategory,
                      )?.children ?? []
                    }
                    selectedElements={
                      new Set(tempSelectedOptions[openCategory])
                    }
                    flipOption={(value) => flipOption(openCategory, value)}
                  />
                </MenuList>
                <Footer onCancelClick={closeMenu} onApplyClick={applyOptions} />
              </Card>
            )}
        </div>
      </div>
    </>
  );
}
