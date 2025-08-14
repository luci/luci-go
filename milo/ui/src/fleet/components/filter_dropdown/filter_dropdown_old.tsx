/* eslint-disable jsx-a11y/no-static-element-interactions */
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
  Divider,
  MenuItem,
  MenuList,
  Skeleton,
  Typography,
} from '@mui/material';
import { useCallback, useEffect, useRef, useState } from 'react';

import { colors } from '@/fleet/theme/colors';
import { SortedElement } from '@/fleet/utils/fuzzy_sort';

import { hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { HighlightCharacter } from '../highlight_character';
import { Footer } from '../options_dropdown/footer';
import { SearchInput } from '../search_input';

export type OptionComponentProps<T> = {
  searchQuery: string;
  optionComponentProps: T;
};

export type OptionComponent<T> = React.FC<OptionComponentProps<T>>;

export type FilterCategoryDataOld<T> = {
  value: string;
  label: string;
  getSearchScore: (searchQuery: string) => {
    score: number;
    matches: number[];
  };
  optionsComponent: OptionComponent<T>;
  optionsComponentProps: T;
};

interface FilterDropdownProps<T> {
  filterOptions: FilterCategoryDataOld<T>[];
  onApply: () => void;
  anchorEl: HTMLElement | null;
  setAnchorEL: React.Dispatch<React.SetStateAction<HTMLElement | null>>;
  isLoading?: boolean;
  commonOptions?: string[];
}

/**
 * @deprecated The old filter dropdown component containing the search bar.
 * Will be replaced after all pages are migrated to go/fleet-console-unified-filter-bar
 */
export function FilterDropdownOld<T>({
  filterOptions,
  onApply,
  anchorEl,
  setAnchorEL,
  isLoading,
  commonOptions,
}: FilterDropdownProps<T>) {
  const [openCategory, setOpenCategory] = useState<
    { value: string; anchor: HTMLElement } | undefined
  >();

  const [searchQuery, setSearchQuery] = useState('');
  const searchInput = useRef<HTMLInputElement>(null);

  useEffect(() => {
    // The autofocus prop is not working
    // https://github.com/mui/material-ui/issues/40397
    if (anchorEl) {
      searchInput.current?.focus();
    }
  }, [anchorEl]);

  const closeInnerMenu = useCallback(() => {
    openCategory?.anchor?.focus();
    setOpenCategory(undefined);
  }, [openCategory]);

  const closeMenu = () => {
    setSearchQuery('');
    closeInnerMenu();
    setAnchorEL(null);
  };

  const applyOptions = () => {
    closeMenu();
    onApply();
  };

  const filterResults = filterOptions
    .map((option) => {
      const searchScore = option.getSearchScore(searchQuery);
      return {
        el: option,
        score: searchScore.score,
        matches: searchScore.matches,
      } as SortedElement<FilterCategoryDataOld<T>>;
    })
    .filter((a) => searchQuery === '' || a.score > 0)
    .sort((a, b) => b.score - a.score);

  const otherFilterResults = commonOptions
    ? filterResults.filter((option) => !commonOptions.includes(option.el.value))
    : filterResults;

  const commonFilterResults =
    commonOptions &&
    filterResults?.filter((option) => commonOptions.includes(option.el.value));

  const handleRandomTextInput: (e: React.KeyboardEvent<HTMLElement>) => void = (
    e: React.KeyboardEvent<HTMLElement>,
  ) => {
    // allow user to search when any alphanumeric key has been pressed
    if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
      searchInput.current?.focus();
      setSearchQuery((old) => old + e.key);
      e.preventDefault(); // Avoid race condition to type twice in the input
    }
  };

  const renderOpenCategory = () => {
    if (!openCategory) return <></>;
    const openCategoryData = filterOptions.find(
      (option) => option.value === openCategory.value,
    )!;
    const OptionComponent = openCategoryData.optionsComponent;
    return (
      <Card onClick={(e) => e.stopPropagation()}>
        <div
          onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
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
          <OptionComponent
            key={openCategoryData.value}
            searchQuery={searchQuery}
            optionComponentProps={openCategoryData.optionsComponentProps}
          />
          <Footer
            onCancelClick={closeMenu}
            onApplyClick={() => {
              applyOptions();
            }}
          />
        </div>
      </Card>
    );
  };

  const renderOption = (
    searchResult: SortedElement<FilterCategoryDataOld<T>>,
  ) => {
    const parent = searchResult;
    const parentMatches = parent.matches;
    return (
      <MenuItem
        onClick={(event) => {
          // The onClick fires also when closing the menu
          if (openCategory?.value !== parent.el.value) {
            setOpenCategory({
              value: parent.el.value,
              anchor: event.currentTarget,
            });
          } else {
            setOpenCategory(undefined);
          }
        }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowRight') {
            e.currentTarget.click();
          }
        }}
        key={`item-${parent.el.value}`}
        disableRipple
        selected={openCategory?.value === parent.el.value}
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
          minHeight: 'auto',
        }}
      >
        <HighlightCharacter variant="body2" highlightIndexes={parentMatches}>
          {parent.el.label}
        </HighlightCharacter>
        <ArrowRightIcon />
      </MenuItem>
    );
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
              maxHeight: 400,
              overflow: 'auto',
              paddingTop: 0,
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
              style={{
                paddingTop: 8,
                marginBottom: '10px',
                position: 'sticky',
                top: 0,
                backgroundColor: colors.white,
                zIndex: 1,
              }}
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
                commonFilterResults && [
                  commonFilterResults.length > 0 && (
                    <Typography
                      tabIndex={-1}
                      variant="caption"
                      color={colors.grey[700]}
                      fontStyle="italic"
                      sx={{
                        paddingLeft: '16px',
                        display: 'block',
                      }}
                      key="common_filters_title"
                    >
                      Common filters
                    </Typography>
                  ),
                  commonFilterResults?.map(renderOption),
                  otherFilterResults.length > 0 &&
                    commonFilterResults.length > 0 && (
                      <Divider
                        sx={{
                          backgroundColor: 'transparent',
                        }}
                        key="common_filters_divider"
                      />
                    ),
                  commonOptions.length > 0 && otherFilterResults.length > 0 && (
                    <Typography
                      variant="caption"
                      color={colors.grey[700]}
                      fontStyle="italic"
                      sx={{
                        margin: '16px',
                      }}
                      key="other_filters_title"
                    >
                      Other filters
                    </Typography>
                  ),
                ],
                otherFilterResults.map(renderOption),
              ]
            )}
          </MenuList>
        </Card>
        <div
          css={{
            position: 'absolute',
            ...getInnerCardRefPositions(anchorEl, openCategory?.anchor),
          }}
        >
          {renderOpenCategory()}
        </div>
      </div>
    </>
  );
}

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
  anchorElInner: HTMLElement | undefined,
) => {
  const anchorRect = anchorEl?.getBoundingClientRect();
  const outerMenuRect = anchorElInner?.getBoundingClientRect();
  const outerMenuParentWidth =
    anchorElInner?.parentElement?.getBoundingClientRect().width;
  if (!anchorRect || !outerMenuRect || !outerMenuParentWidth) return;
  const anchorParentRect = anchorEl?.parentElement?.getBoundingClientRect();
  const newInnerCardRefPosition = {
    left: `${anchorRect.left - (anchorParentRect?.left || 0) + outerMenuParentWidth}px`,
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
