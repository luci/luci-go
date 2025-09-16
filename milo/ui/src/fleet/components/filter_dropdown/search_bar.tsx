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
import { forwardRef, useRef } from 'react';

import { colors } from '@/fleet/theme/colors';
import { keyboardUpDownHandler } from '@/fleet/utils';

import { FilterCategoryData } from './filter_dropdown';
import { SelectedChip } from './selected_chip';

interface SearchBarProps<T> {
  value: string;
  onChange: (value: string) => void;
  selectedOptions: FilterCategoryData<T>[];
  onDropdownFocus: () => void;
  onChangeDropdownOpen: (isOpen: boolean) => void;
  onChipDeleted: (option: FilterCategoryData<T>) => void;
  getLabel: (option: FilterCategoryData<T>) => string; // TODO: could be part of FilterCategoryData itself
  isLoading?: boolean;
  onChipEditApplied: () => void;
}

export const SearchBar = forwardRef<HTMLInputElement, SearchBarProps<unknown>>(
  function SearchBar<T>(
    {
      value,
      onChange,
      selectedOptions,
      onDropdownFocus,
      onChangeDropdownOpen,
      getLabel,
      onChipDeleted,
      onChipEditApplied,
    }: SearchBarProps<T>,
    ref: React.ForwardedRef<HTMLInputElement>,
  ) {
    const internalRef = useRef<HTMLInputElement>(null);

    const chipListRef = useRef<(HTMLDivElement | null)[]>([]);

    const renderChip = (option: FilterCategoryData<unknown>, i: number) => {
      const OptionComponent = option.optionsComponent;

      return (
        <SelectedChip
          dropdownContent={(searchQuery, onNavigateUp) => (
            <OptionComponent
              childrenSearchQuery={searchQuery}
              optionComponentProps={option.optionsComponentProps}
              onNavigateUp={onNavigateUp}
            />
          )}
          ref={(el) => {
            chipListRef.current[i] = el;
            chipListRef.current = chipListRef.current.filter((x) => x !== null);
          }}
          onFocus={() => {
            // close the dropdown on text field blur
            // onBlur is not optimal, as it triggers when dropdown is clicked, so we change it manually here
            onChangeDropdownOpen(false);
          }}
          onClick={(e) => {
            e.stopPropagation();
          }}
          onBlur={(e) => {
            e.stopPropagation();
          }}
          onKeyDown={(e) => {
            // stops search bar from taking over
            e.stopPropagation();

            const currentIndex = chipListRef.current.findIndex(
              (x) => x === e.target,
            );
            if (currentIndex < 0) return; // not found
            if (e.key === 'ArrowLeft') {
              if (currentIndex > 0) {
                chipListRef.current[currentIndex - 1]?.focus();
              }
              e.preventDefault();
            }
            if (e.key === 'ArrowRight') {
              if (currentIndex === chipListRef.current.length - 1) {
                internalRef.current?.focus();
              } else {
                chipListRef.current[currentIndex + 1]?.focus();
              }
              e.preventDefault();
            }
          }}
          onDelete={(e) => {
            onChipDeleted(option as FilterCategoryData<T>);
            if (e.type === 'keyup') {
              // if clicked with a mouse we don't want to focus on the input
              internalRef.current?.focus();
            }
          }}
          key={option.value}
          label={getLabel(option as FilterCategoryData<T>)}
          onApply={onChipEditApplied}
        />
      );
    };

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
            onChangeDropdownOpen(false);
          }
          if (
            e.key === 'Backspace' &&
            internalRef.current?.selectionStart === 0 &&
            internalRef.current?.selectionEnd === 0
          ) {
            if (selectedOptions.length > 0) {
              onChipDeleted(selectedOptions[selectedOptions.length - 1]);
              e.preventDefault();
            }
          }
          if (
            e.key === 'ArrowLeft' &&
            internalRef.current?.selectionStart === 0
          ) {
            // get last chip
            const lastChip =
              chipListRef.current[chipListRef.current.length - 1];
            if (lastChip) {
              lastChip?.focus();
              e.preventDefault();
            }
          }
          keyboardUpDownHandler(e, () => {
            onDropdownFocus();
            e.preventDefault();
          });
        }}
        placeholder='Add a filter (e.g. "dut1" or "state:ready")'
        autoComplete="off"
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
              <>
                {selectedOptions?.map((option, i) =>
                  renderChip(option as FilterCategoryData<unknown>, i),
                )}
                {/*
                     spacer, aligned to the input, allows for correct ref positioning in situations
                     where the input is moved to next line due to wrapping. There might be a cleaner
                     way to do it though */}
                <div css={{ display: 'inline' }} />
              </>
            ),
          },
        }}
      />
    );
  },
);
