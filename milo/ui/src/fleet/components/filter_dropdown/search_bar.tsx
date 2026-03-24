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

import { TextField } from '@mui/material';
import { forwardRef, useCallback, useMemo, useRef } from 'react';

import { colors } from '@/fleet/theme/colors';
import { keyboardListNavigationHandler } from '@/fleet/utils';

import { StringListFilterCategory } from '../filters/string_list_filter';
import { FilterCategory } from '../filters/use_filters';

import { SelectedChip } from './selected_chip';

interface SearchBarProps {
  value: string;
  onChange: (value: string) => void;
  filterCategoryDatas: FilterCategory[];

  onDropdownFocus: () => void;
  isDropdownOpen: boolean;
  onChangeDropdownOpen: (isOpen: boolean) => void;
  isLoading?: boolean;
  onChipEditApplied: () => void;
  placeholder?: string;
}

export const SearchBar = forwardRef<HTMLInputElement, SearchBarProps>(
  function SearchBar(
    {
      value,
      onChange,
      filterCategoryDatas,
      onDropdownFocus,
      isDropdownOpen,
      onChangeDropdownOpen,
      onChipEditApplied,
      placeholder = 'Add a filter',
    }: SearchBarProps,
    ref: React.ForwardedRef<HTMLInputElement>,
  ) {
    const internalRef = useRef<HTMLInputElement>(null);

    const chipListRef = useRef<(HTMLDivElement | null)[]>([]);

    const renderChip = useCallback(
      function renderChip(option: FilterCategory, i: number) {
        return (
          <SelectedChip
            filterCategory={option}
            key={`renderChip-${option.key}`}
            enableSearchInput={option instanceof StringListFilterCategory}
            ref={(el) => {
              chipListRef.current[i] = el;
              chipListRef.current = chipListRef.current.filter(
                (x) => x !== null,
              );
            }}
            onFocus={() => {
              // close the dropdown on text field blur
              // onBlur is not optimal, as it triggers when dropdown is clicked, so we change it manually here
              onChangeDropdownOpen(false);
            }}
            onBlur={(e) => {
              e.stopPropagation();
            }}
            onDelete={(e) => {
              option.clear();
              if (e.type === 'keyup') {
                // if clicked with a mouse we don't want to focus on the input
                internalRef.current?.focus();
              }
            }}
            label={option.getChipLabel()}
            onApply={onChipEditApplied}
          />
        );
      },
      [onChangeDropdownOpen, onChipEditApplied],
    );

    const renderedChips = useMemo(
      () =>
        (filterCategoryDatas ?? [])
          .filter((fcd) => fcd.isActive())
          ?.map((option, i) => renderChip(option, i)),
      [filterCategoryDatas, renderChip],
    );

    return (
      <TextField
        data-testid="search-bar"
        sx={{
          width: '100%',
          zIndex: 3, // above the filter dropdown backdrop
        }}
        inputRef={(node) => {
          internalRef.current = node;
          if (ref) {
            if (typeof ref === 'function') {
              // If it's a callback ref, call it with the node.
              ref(node);
            } else {
              // If it's a ref object, assign the node to its .current property.
              ref.current = node;
            }
          }
        }}
        value={value}
        onChange={(e) => {
          onChange(e.target.value);
        }}
        onClick={() => {
          onChangeDropdownOpen(true);
        }}
        onFocus={() => {
          onChangeDropdownOpen(true);
        }}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            if (!isDropdownOpen) {
              (e.target as HTMLElement).blur();
            }
            onChangeDropdownOpen(false);
          }
          if (
            e.key === 'Backspace' &&
            internalRef.current?.selectionStart === 0 &&
            internalRef.current?.selectionEnd === 0
          ) {
            const lastSelectedFilter = filterCategoryDatas
              .filter((fcd) => fcd.isActive())
              .at(-1);
            if (lastSelectedFilter) {
              lastSelectedFilter.clear();
              e.preventDefault();
            }
          }
          keyboardListNavigationHandler(
            e,
            () => {
              onDropdownFocus();
              e.preventDefault();
            },
            undefined,
            'vertical',
          );
          keyboardListNavigationHandler(
            e,
            undefined,
            internalRef.current?.selectionStart === 0
              ? () => {
                  // get last chip
                  const lastChip =
                    chipListRef.current[chipListRef.current.length - 1];
                  if (lastChip) {
                    lastChip?.focus();
                    e.preventDefault();
                  }
                }
              : undefined,
            'horizontal',
          );
        }}
        placeholder={placeholder}
        size="small"
        slotProps={{
          input: {
            sx: {
              width: '100%',
              display: 'inline-flex',
              flexWrap: 'wrap',
              padding: '2px 4px',
              fontSize: 14,

              '& input': {
                marginLeft: '10px',
                minWidth: '100px',
                flex: 1,
                padding: '8px 4px',
              },
              '& fieldset': {
                borderColor: colors.grey[300],
              },

              '&:hover fieldset': {
                borderColor: colors.grey[500] + '!important',
              },

              '&:focus-within fieldset': {
                borderWidth: '1px !important',
              },
            },
            startAdornment: (
              // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions
              <div
                onClick={(e) => e.stopPropagation()}
                style={{ display: 'contents' }}
              >
                {renderedChips}
                {/*
                     spacer, aligned to the input, allows for correct ref positioning in situations
                     where the input is moved to next line due to wrapping. There might be a cleaner
                     way to do it though */}
                <div css={{ display: 'inline' }} />
              </div>
            ),
          },
        }}
      />
    );
  },
);
