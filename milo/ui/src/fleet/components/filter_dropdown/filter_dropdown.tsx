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
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from 'react';

import { colors } from '@/fleet/theme/colors';
import { hasAnyModifier, keyboardListNavigationHandler } from '@/fleet/utils';
import { fuzzySubstring, SortedElement } from '@/fleet/utils/fuzzy_sort';

import { HighlightCharacter } from '../highlight_character';
import { Footer } from '../options_dropdown/footer';

export interface OptionComponentHandle {
  focus: () => void;
}

export type OptionComponentProps<T> = {
  childrenSearchQuery: string;
  onNavigateUp?: (e: React.KeyboardEvent) => void;
  optionComponentProps: T;
};

export type OptionComponent<T> = React.ForwardRefExoticComponent<
  OptionComponentProps<T> & React.RefAttributes<OptionComponentHandle>
>;

export type FilterCategoryData<T> = {
  value: string;
  label: string;
  getChildrenSearchScore: (childrenSearchQuery: string) => number;
  optionsComponent: OptionComponent<T>;
  optionsComponentProps: T;
};

interface FilterDropdownProps<T> {
  searchQuery: string;
  onSearchQueryChange: (searchQuery: string) => void;
  onSearchBarFocus: () => void;
  filterOptions: FilterCategoryData<T>[];
  onApply: () => void;
  anchorEl: HTMLElement | null;
  onClose: () => void;
  isLoading?: boolean;
  commonOptions?: string[];
  categoryValueSeparator?: string;
}

export interface FilterDropdownHandle {
  focus: () => void;
}

// randomly selected multiplier, seems to work well
const PARENT_SEARCH_SCORE_MULTIPLIER = 1.05;

export const FilterDropdown = forwardRef(function FilterDropdownNew<T>(
  {
    searchQuery,
    onSearchQueryChange,
    onSearchBarFocus,
    filterOptions,
    onApply,
    anchorEl,
    onClose,
    isLoading,
    commonOptions,
    categoryValueSeparator = ':',
  }: FilterDropdownProps<T>,
  ref: React.ForwardedRef<FilterDropdownHandle>,
) {
  const [openCategory, setOpenCategory] = useState<
    { value: string; anchor: HTMLElement } | undefined
  >();

  const openCategoryRef = useRef<OptionComponentHandle>(null);
  const firstElementRef = useRef<HTMLLIElement>(null);

  useImperativeHandle(ref, () => ({
    focus: () => {
      if (openCategory) {
        openCategoryRef.current?.focus();
      } else {
        firstElementRef.current?.focus();
      }
    },
  }));

  useEffect(() => {
    if (openCategory) {
      // The component for the open category might forward its `focus`
      // method to a child element using `useImperativeHandle`. This can create a
      // race condition where this `useEffect` hook runs before the child component's refs are mounted.
      // `setTimeout` with a 0ms delay defers the `focus()` call to the next
      // event loop, ensuring the child component has mounted and the ref is set.
      setTimeout(() => {
        openCategoryRef.current?.focus();
      });
    }
  }, [openCategory]);

  useEffect(() => {
    if (!anchorEl) {
      closeInnerMenu();
    }
  }, [anchorEl]);

  const closeInnerMenu = () => {
    setOpenCategory(undefined);
  };

  const closeMenu = () => {
    closeInnerMenu();
    onClose();
  };

  const applyOptions = () => {
    closeMenu();
    onApply();
  };

  const splitSearchQuery = (
    searchQuery: string,
  ): {
    isCategoryScoped: boolean;
    parentSearchQuery: string;
    childrenSearchQuery: string;
  } => {
    const parts = searchQuery.split(categoryValueSeparator);
    if (parts.length > 1) {
      return {
        isCategoryScoped: true,
        parentSearchQuery: parts[0],
        childrenSearchQuery: parts[1],
      };
    } else {
      return {
        // if search query has no separator, we want to run the filter on both parent and children
        isCategoryScoped: false,
        parentSearchQuery: searchQuery,
        childrenSearchQuery: searchQuery,
      };
    }
  };

  const filterResults = filterOptions
    .map((option) => {
      const { parentSearchQuery, childrenSearchQuery } =
        splitSearchQuery(searchQuery);

      const childrenScore = option.getChildrenSearchScore(
        childrenSearchQuery.trim(),
      );

      const parentScore = fuzzySubstring(parentSearchQuery, option.label);

      return {
        el: option,
        score: Math.max(
          // will prioritize parent matches when children have similar scores
          // (e.g. "id" search will prioritize "dut id" category over "label-xyz" category with value "someid")
          parentScore[0] * PARENT_SEARCH_SCORE_MULTIPLIER,
          childrenScore,
        ),
        matches: parentScore[1],
      } as SortedElement<FilterCategoryData<T>>;
    })
    .filter((a) => searchQuery.trim() === '' || a.score > 0)
    .sort((a, b) => b.score - a.score);

  if (
    openCategory &&
    !filterResults.find((result) => result.el.value === openCategory.value)
  ) {
    closeInnerMenu();
  }

  const otherFilterResults = commonOptions
    ? filterResults.filter((option) => !commonOptions.includes(option.el.value))
    : filterResults;

  const commonFilterResults =
    commonOptions &&
    filterResults?.filter((option) => commonOptions.includes(option.el.value));

  const onSearchQueryChangeInternal = (newValue: string) => {
    const { isCategoryScoped } = splitSearchQuery(newValue);

    if (isCategoryScoped) {
      // already scoped to category, no autocomplete to do
      onSearchQueryChange(newValue);
      return;
    }

    if (openCategory && newValue.length > searchQuery.length) {
      const openCategoryName = filterOptions.find(
        (option) => option.value === openCategory?.value,
      )?.label;

      onSearchQueryChange(
        openCategoryName +
          categoryValueSeparator +
          newValue[newValue.length - 1],
      );
      return;
    }
    onSearchQueryChange(newValue);
  };

  const handleRandomTextInput: (e: React.KeyboardEvent<HTMLElement>) => void = (
    e: React.KeyboardEvent<HTMLElement>,
  ) => {
    // allows for 'select all' (ctrl/cmd + a)
    if (hasAnyModifier(e) && e.key === 'a') {
      onSearchBarFocus();
      return;
    }
    if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
      // allow user to search when any alphanumeric key has been pressed
      onSearchBarFocus();
      onSearchQueryChangeInternal(searchQuery + e.key);
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
            if (e.key === 'Escape') {
              openCategory.anchor.focus();
              closeInnerMenu();
            }
            if (e.key === 'Enter' && e.ctrlKey) {
              applyOptions();
            }
            if (e.key === 'Backspace') {
              onSearchQueryChangeInternal(
                searchQuery.slice(0, searchQuery.length - 1),
              );
              onSearchBarFocus();
              e.preventDefault();
            }
            if (e.key === 'Delete' || e.key === 'Cancel') {
              onSearchQueryChangeInternal('');
              onSearchBarFocus();
            }

            keyboardListNavigationHandler(
              e,
              undefined,
              () => {
                openCategory.anchor.focus();
                closeInnerMenu();
              },
              'horizontal',
            );

            handleRandomTextInput(e);
          }}
        >
          <OptionComponent
            ref={openCategoryRef}
            key={openCategoryData.value}
            childrenSearchQuery={
              splitSearchQuery(searchQuery).childrenSearchQuery
            }
            optionComponentProps={openCategoryData.optionsComponentProps}
            onNavigateUp={() => {
              onSearchBarFocus();
            }}
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
    searchResult: SortedElement<FilterCategoryData<T>>,
    isFirstOption: boolean,
  ) => {
    const parent = searchResult;
    const parentMatches = parent.matches;

    const refProps = isFirstOption ? { ref: firstElementRef } : {};

    return (
      <MenuItem
        {...refProps}
        onClick={(event) => {
          // The onClick fires also when closing the menu
          if (openCategory?.value !== parent.el.value) {
            setOpenCategory({
              value: parent.el.value,
              anchor: event.currentTarget,
            });
          } else {
            closeInnerMenu();
          }
        }}
        onKeyDown={(e) => {
          keyboardListNavigationHandler(
            e,
            () => {
              if (openCategory?.value === parent.el.value) {
                openCategoryRef.current?.focus();
              } else {
                setOpenCategory({
                  value: parent.el.value,
                  anchor: e.currentTarget,
                });
              }
            },
            () => {
              closeInnerMenu();
            },
            'horizontal',
          );
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
        data-testid="filter-dropdown-backdrop"
        open={!!anchorEl}
        onClick={() => {
          closeMenu();
        }}
        invisible={true}
        sx={{
          zIndex: 2,
        }}
      ></Backdrop>
      {anchorEl && (
        <div
          css={{
            zIndex: 3,
            position: 'absolute',
            marginTop: 3,
            display: 'block',
          }}
        >
          <Card
            elevation={2}
            onClick={(e) => e.stopPropagation()}
            sx={{ position: 'absolute', ...getCardRefPosition(anchorEl) }}
          >
            <MenuList
              sx={{
                minWidth: 300,
                maxHeight: 400,
                overflow: 'auto',
              }}
              onKeyDown={(e) => {
                const isFirstElement = e.target === firstElementRef.current;

                keyboardListNavigationHandler(
                  e,
                  undefined,
                  isFirstElement
                    ? () => {
                        onSearchBarFocus();
                        e.preventDefault();
                        e.stopPropagation();
                      }
                    : undefined,
                );
                switch (e.key) {
                  case 'Delete':
                  case 'Cancel':
                  case 'Backspace':
                    onSearchQueryChangeInternal('');
                    onSearchBarFocus();
                    break;
                  case 'Escape':
                    onSearchBarFocus();
                    closeMenu();
                    break;
                }

                handleRandomTextInput(e);
              }}
            >
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
              ) : filterResults.length > 0 ? (
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
                    commonFilterResults?.map((filterResult, index) =>
                      renderOption(filterResult, index === 0),
                    ),

                    otherFilterResults.length > 0 &&
                      commonFilterResults.length > 0 && (
                        <Divider
                          sx={{
                            backgroundColor: 'transparent',
                          }}
                          key="common_filters_divider"
                        />
                      ),

                    commonOptions.length > 0 &&
                      otherFilterResults.length > 0 && (
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
                  otherFilterResults.map((filterResult, index) =>
                    renderOption(
                      filterResult,
                      index === 0 && commonFilterResults?.length === 0,
                    ),
                  ),
                ]
              ) : (
                <div
                  css={{
                    width: '100%',
                    height: '100%',
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    padding: '16px',
                    boxSizing: 'border-box',
                    color: colors.grey[600],
                  }}
                >
                  No results
                </div>
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
      )}
    </>
  );
});

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
