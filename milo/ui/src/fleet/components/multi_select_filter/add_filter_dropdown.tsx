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
import {
  Backdrop,
  Box,
  Card,
  MenuItem,
  MenuList,
  Skeleton,
} from '@mui/material';
import _ from 'lodash';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

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
  onSelectedOptionsChange,
  anchorEl,
  setAnchorEL,
  isLoading,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
  isLoading?: boolean;
}) {
  const [anchorElInner, setAnchorELInner] = useState<HTMLElement | null>(null);
  const [openCategory, setOpenCategory] = useState<string | undefined>();
  const [searchQuery, setSearchQuery] = useState('');
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(initSelectedOptions);
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // The autofocus prop is not working
    // https://github.com/mui/material-ui/issues/40397
    if (anchorEl) {
      searchInput.current?.focus();
    }
  }, [anchorEl]);

  useEffect(() => {
    setTempSelectedOptions(initSelectedOptions);
  }, [initSelectedOptions, openCategory]);

  const closeInnerMenu = useCallback(() => {
    setOpenCategory(undefined);
    anchorElInner?.focus();
    setAnchorELInner(null);
  }, [anchorElInner]);

  const closeMenu = () => {
    setSearchQuery('');
    closeInnerMenu();
    setAnchorEL(null);
  };

  const applyOptions = () => {
    closeMenu();
    onSelectedOptionsChange(tempSelectedOptions);
  };

  const filterResults = useMemo(() => {
    const newFilterResults = updateFilterResults(filterOptions, searchQuery);
    if (
      newFilterResults.filter((f) => f.parent.el.value === openCategory)
        .length === 0
    ) {
      closeInnerMenu();
    }

    return newFilterResults;
  }, [closeInnerMenu, filterOptions, openCategory, searchQuery]);

  const handleRandomTextInput: (
    e: React.KeyboardEvent<HTMLUListElement>,
  ) => void = (e: React.KeyboardEvent<HTMLUListElement>) => {
    // allow user to search when any alphanumeric key has been pressed
    if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
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
          sx={{ position: 'absolute', ...getCardRefPosition(anchorEl) }}
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
              key="search-input"
            />
            {isLoading ? (
              <Box
                sx={{
                  height: 300,
                  width: 258,
                  px: '10px',
                }}
              >
                {Array.from({ length: 8 }).map((_, index) => (
                  <Skeleton
                    key={`item-skeleton-${index}`}
                    variant="text"
                    height={36}
                    sx={{ width: '100%' }}
                  />
                ))}
              </Box>
            ) : (
              [
                filterResults.map((searchResult) => {
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
                      key={`item-${parent.el.value}`}
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
                }),
              ]
            )}
          </MenuList>
        </Card>
        <div
          css={{
            position: 'absolute',
            ...getInnerCardRefPositions(anchorEl, anchorElInner, openCategory),
          }}
        >
          {anchorElInner && openCategory !== undefined && (
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
                  selectedElements={new Set(tempSelectedOptions[openCategory])}
                  flipOption={(value) =>
                    flipOption(
                      openCategory,
                      value,
                      tempSelectedOptions,
                      setTempSelectedOptions,
                    )
                  }
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

const flipOption = (
  parentValue: string,
  o2Value: string,
  tempSelectedOptions: SelectedOptions,
  setTempSelectedOptions: React.Dispatch<React.SetStateAction<SelectedOptions>>,
) => {
  const currentValues = tempSelectedOptions[parentValue] ?? [];

  const newValues = currentValues.includes(o2Value)
    ? currentValues.filter((v) => v !== o2Value)
    : currentValues.concat(o2Value);

  setTempSelectedOptions({
    ...(tempSelectedOptions ?? {}),
    [parentValue]: newValues,
  });
};

const getCardRefPosition = (anchorEl: HTMLElement | null) => {
  const anchorRect = anchorEl?.getBoundingClientRect();
  const anchorParentRect = anchorEl?.parentElement?.getBoundingClientRect();
  if (!anchorRect || !anchorParentRect) return {};

  return {
    left: `${anchorRect.left - (anchorParentRect?.left || 0)}px`,
    top: `${anchorEl?.parentElement?.getBoundingClientRect().height || anchorRect.height}px`,
  };
};

const getInnerCardRefPositions = (
  anchorEl: HTMLElement | null,
  anchorElInner: HTMLElement | null,
  openCategory: string | undefined,
) => {
  const anchorRect = anchorEl?.getBoundingClientRect();
  const outerMenuRect = anchorElInner?.getBoundingClientRect();

  if (!anchorRect) return;
  if (!outerMenuRect) return;
  if (!openCategory) return;

  const anchorParentRect = anchorEl?.parentElement?.getBoundingClientRect();

  const newInnerCardRefPosition = {
    openCategory,
    left: `${anchorRect.left - (anchorParentRect?.left || 0) + outerMenuRect.width + 15}px`,
    top: '',
    bottom: '',
  };
  if (outerMenuRect.top > window.innerHeight / 2) {
    newInnerCardRefPosition.top = '';
    newInnerCardRefPosition.bottom = `${-outerMenuRect.bottom + anchorRect.top - 30}px`;
  } else {
    newInnerCardRefPosition.top = `${outerMenuRect.top - anchorRect.top - 30}px`;
    newInnerCardRefPosition.bottom = '';
  }
  return newInnerCardRefPosition;
};

const updateFilterResults = (
  filterOptions: OptionCategory[],
  searchQuery: string,
) => {
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
      g.parent.score > 0 || (g.children.length > 0 && g.children[0].score > 0),
  );
};
