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
  Box,
  ClickAwayListener,
  Divider,
  MenuItem,
  MenuList,
  Paper,
  Popper,
  Skeleton,
  Typography,
} from '@mui/material';
import {
  forwardRef,
  useCallback,
  useDeferredValue,
  useEffect,
  useImperativeHandle,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { colors } from '@/fleet/theme/colors';
import { hasAnyModifier, keyboardListNavigationHandler } from '@/fleet/utils';
import { fuzzySubstring, SortedElement } from '@/fleet/utils/fuzzy_sort';

import { EllipsisTooltip } from '../ellipsis_tooltip';
import { StringListFilterCategory } from '../filters/string_list_filter';
import { FilterCategory } from '../filters/use_filters';
import { HighlightCharacter } from '../highlight_character';

export interface OptionComponentHandle {
  focus: () => void;
}

export type OptionComponentProps<T> = {
  childrenSearchQuery: string;
  onNavigateUp?: (e: React.KeyboardEvent) => void;
  maxHeight?: number;
  optionComponentProps: T;
};

export type OptionComponent<T> = React.ForwardRefExoticComponent<
  OptionComponentProps<T> & React.RefAttributes<OptionComponentHandle>
>;

interface FilterDropdownProps {
  searchQuery: string;
  onSearchQueryChange: (searchQuery: string) => void;
  onSearchBarFocus: () => void;
  filterCategoryDatas: FilterCategory[];
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

export const FilterDropdown = forwardRef(function FilterDropdownNew(
  {
    searchQuery,
    onSearchQueryChange,
    onSearchBarFocus,
    filterCategoryDatas,
    onApply,
    anchorEl,
    onClose,
    isLoading,
    commonOptions,
    categoryValueSeparator = ':',
  }: FilterDropdownProps,
  ref: React.ForwardedRef<FilterDropdownHandle>,
) {
  const [openCategory, setOpenCategory] = useState<
    { value: FilterCategory; anchor: HTMLElement } | undefined
  >();
  useLayoutEffect(() => {
    // if the current open category is filterd out by the search it should no longer be open
    if (openCategory && !openCategory.anchor.isConnected)
      setOpenCategory(undefined);
  }, [openCategory, searchQuery]);

  const deferredSearchQuery = useDeferredValue(searchQuery);
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
      setOpenCategory(undefined);
    }
  }, [anchorEl]);

  const closeInnerMenu = () => {
    setOpenCategory(undefined);
    // When closing the inner menu, we want to refocus the item in the main menu
    // to allow keyboard navigation to continue
    if (openCategory?.anchor) {
      openCategory.anchor.focus();
    }
  };

  const closeMenu = () => {
    closeInnerMenu();
    onClose();
  };

  const applyOptions = () => {
    closeMenu();
    onApply();
  };

  const splitSearchQuery = useCallback(
    (
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
          childrenSearchQuery: parts.slice(1).join(categoryValueSeparator),
        };
      } else {
        return {
          // if search query has no separator, we want to run the filter on both parent and children
          isCategoryScoped: false,
          parentSearchQuery: searchQuery,
          childrenSearchQuery: searchQuery,
        };
      }
    },
    [categoryValueSeparator],
  );

  const filterResults = useMemo(
    () =>
      filterCategoryDatas
        .map((option) => {
          const { parentSearchQuery, childrenSearchQuery } =
            splitSearchQuery(deferredSearchQuery);

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
          };
        })
        .filter((a) => deferredSearchQuery.trim() === '' || a.score > 0)
        .sort((a, b) => b.score - a.score),
    [deferredSearchQuery, filterCategoryDatas, splitSearchQuery],
  );

  const otherFilterResults = commonOptions
    ? filterResults.filter((option) => !commonOptions.includes(option.el.key))
    : filterResults;

  const commonFilterResults =
    commonOptions &&
    filterResults?.filter((option) => commonOptions.includes(option.el.key));

  const onSearchQueryChangeInternal = (newValue: string) => {
    const { isCategoryScoped } = splitSearchQuery(newValue);

    if (isCategoryScoped) {
      // already scoped to category, no autocomplete to do
      onSearchQueryChange(newValue);
      return;
    }

    if (openCategory && newValue.length > searchQuery.length) {
      const openCategoryName = openCategory.value.label;

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

    const content = openCategory.value.render(
      splitSearchQuery(deferredSearchQuery).childrenSearchQuery,
      () => {
        onSearchBarFocus();
      },
      applyOptions,
      closeMenu,
      openCategoryRef,
    );

    return (
      <Popper
        open={true}
        anchorEl={openCategory.anchor}
        placement="right-start"
        style={{ zIndex: 1301 }}
        modifiers={[
          {
            name: 'eventListeners',
            enabled: true,
            options: {
              scroll: false,
              resize: true,
            },
          },
          {
            name: 'flip',
            enabled: true,
            options: {
              boundary: 'viewport',
            },
          },
          {
            name: 'preventOverflow',
            enabled: true,
            options: {
              boundary: 'viewport',
            },
          },
        ]}
      >
        <Paper
          elevation={2}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
            if (openCategory?.value instanceof StringListFilterCategory) {
              if (e.key === 'Backspace') {
                onSearchQueryChangeInternal(
                  searchQuery.slice(0, searchQuery.length - 1),
                );
                onSearchBarFocus();
                e.preventDefault();
                return;
              }
              if (e.key === 'Delete' || e.key === 'Cancel') {
                onSearchQueryChangeInternal('');
                onSearchBarFocus();
                e.preventDefault();
                return;
              }
            }
            if (e.key === 'Tab') {
              closeMenu();
              return;
            }
            if (e.key === 'Escape') {
              openCategory?.anchor.focus();
              closeInnerMenu();
              return;
            }

            if (openCategory?.value instanceof StringListFilterCategory) {
              handleRandomTextInput(e);
              keyboardListNavigationHandler(
                e,
                undefined,
                () => {
                  openCategory?.anchor.focus();
                  closeInnerMenu();
                },
                'horizontal',
              );
            }
          }}
        >
          {content}
        </Paper>
      </Popper>
    );
  };

  const renderOption = (
    searchResult: SortedElement<FilterCategory>,
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
          if (openCategory?.value !== parent.el) {
            setOpenCategory({
              value: parent.el,
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
              if (openCategory?.value === parent.el) {
                openCategoryRef.current?.focus();
              } else {
                setOpenCategory({
                  value: parent.el,
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
        key={`item-${parent.el.key}`}
        disableRipple
        selected={openCategory?.value === parent.el}
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
          minHeight: 'auto',
        }}
      >
        <EllipsisTooltip tooltip={parent.el.label}>
          <HighlightCharacter
            variant="body2"
            highlightIndexes={parentMatches}
            sx={{
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {parent.el.label}
          </HighlightCharacter>
        </EllipsisTooltip>
        <ArrowRightIcon />
      </MenuItem>
    );
  };

  return (
    <>
      <Popper
        open={!!anchorEl}
        anchorEl={anchorEl}
        placement="bottom-start"
        style={{ zIndex: 1300 }}
        modifiers={[
          {
            name: 'offset',
            options: {
              offset: [0, 8],
            },
          },
        ]}
      >
        <ClickAwayListener
          onClickAway={(e) => {
            if (anchorEl && anchorEl.contains(e.target as Node)) {
              return;
            }
            closeMenu();
          }}
        >
          <Paper elevation={2}>
            <MenuList
              sx={{
                minWidth: 300,
                maxHeight: 400,
                overflow: 'auto',
                maxWidth: '700px',
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
          </Paper>
        </ClickAwayListener>
      </Popper>
      {renderOpenCategory()}
    </>
  );
});
