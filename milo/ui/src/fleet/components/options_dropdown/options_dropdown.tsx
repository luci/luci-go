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

import {
  Box,
  Menu,
  MenuItem,
  MenuList,
  MenuProps,
  PopoverOrigin,
  Skeleton,
} from '@mui/material';
import { useEffect, useMemo, useRef, useState } from 'react';

import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { OptionsMenu } from '../filter_dropdown/options_menu';
import { SearchInput } from '../search_input';

import { Footer } from './footer';

type OptionsDropdownProps = MenuProps & {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  anchorEl: HTMLElement | null;
  open: boolean;
  option: OptionCategory;
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange?: (newSelectedOptions: SelectedOptions) => void;
  anchorOrigin?: PopoverOrigin | undefined;
  highlightedCharacters?: Record<string, number[]>;
  disableFooter?: boolean;
  enableSearchInput?: boolean;
  onFlipOption?: (value: string) => void;
  maxHeight?: number;
  isLoading?: boolean;
};

function MenuSkeleton({
  itemCount,
  maxHeight,
  disableFooter,
}: {
  itemCount: number;
  maxHeight: number;
  disableFooter: boolean;
}) {
  return (
    <Box key="menu-container-skeleton">
      {' '}
      <MenuList
        sx={{
          overflowY: 'auto',
        }}
        tabIndex={-1}
        key="menu-skeleton"
      >
        <Box
          sx={{
            width: 280,
            maxHeight: maxHeight,
            px: '10px',
          }}
          key="options-container-skeleton"
        >
          {Array.from({ length: itemCount }).map((_, index) => (
            <MenuItem
              sx={{
                height: 30,
                display: 'flex',
                alignItems: 'center',
                padding: '4px 0',
              }}
              key={`option-${index}-skeleton`}
            >
              <Skeleton
                variant="rectangular"
                sx={{
                  width: 20,
                  height: 20,
                  marginRight: 1,
                }}
              />
              <Skeleton
                variant="text"
                height={32}
                sx={{ width: '100%', marginBottom: '1px' }}
              />
            </MenuItem>
          ))}
        </Box>
      </MenuList>
      {!disableFooter && (
        <Box
          sx={{
            height: 30,
            display: 'flex',
            padding: 2,
            justifyContent: 'space-between',
          }}
          key="menu-footer-skeleton"
        >
          <Skeleton
            variant="rectangular"
            width={80}
            height={36}
            key="menu-footer-cancel-skeleton"
          />
          <Skeleton
            variant="rectangular"
            width={80}
            height={36}
            key="menu-footer-apply-skeleton"
          />
        </Box>
      )}
    </Box>
  );
}

export function OptionsDropdown({
  onClose,
  anchorEl,
  open,
  option,
  selectedOptions,
  onSelectedOptionsChange,
  anchorOrigin = {
    vertical: 'top',
    horizontal: 'right',
  },
  onKeyDown,
  highlightedCharacters,
  disableFooter = false,
  enableSearchInput = false,
  onFlipOption,
  maxHeight = 200,
  isLoading,
  ...menuProps
}: OptionsDropdownProps) {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(selectedOptions);

  useEffect(() => {
    if (open) {
      setTempSelectedOptions(selectedOptions);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const flipOption = (o2Value: string) => {
    const currentValues = tempSelectedOptions[option.value] ?? [];

    const newValues = currentValues.includes(o2Value)
      ? currentValues.filter((v) => v !== o2Value)
      : currentValues.concat(o2Value);

    setTempSelectedOptions({
      ...(tempSelectedOptions ?? {}),
      [option.value]: newValues,
    });

    if (onFlipOption) onFlipOption(o2Value);
  };

  const resetTempOptions = () => setTempSelectedOptions(selectedOptions);
  const confirmTempOptions = () => {
    if (onSelectedOptionsChange) {
      onSelectedOptionsChange(tempSelectedOptions);
    }
    if (onClose) onClose({}, 'backdropClick');
  };

  const searchInput = useRef<HTMLInputElement>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const [options, highlightedCharactersWrapper] = useMemo(() => {
    if (enableSearchInput && searchQuery !== '') {
      const results = fuzzySort(searchQuery)(
        option.options,
        (el) => el.label,
      ).filter((sr) => sr.score > 0);

      const highlightedCharacters = Object.fromEntries(
        results.map((sr) => [sr.el.value, sr.matches]),
      );
      return [
        results.map((sr) => ({ label: sr.el.label, value: sr.el.value })),
        highlightedCharacters,
      ];
    } else {
      return [option.options, highlightedCharacters];
    }
  }, [option.options, searchQuery, highlightedCharacters, enableSearchInput]);

  return (
    <Menu
      variant="selectedMenu"
      onClose={(...args) => {
        if (onClose) onClose(...args);
        resetTempOptions();
        if (enableSearchInput) {
          setSearchQuery('');
        }
      }}
      open={open}
      anchorEl={anchorEl}
      anchorOrigin={anchorOrigin}
      elevation={2}
      onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Enter' && e.ctrlKey) {
          confirmTempOptions();
        }

        if (enableSearchInput) {
          keyboardUpDownHandler(e);
          switch (e.key) {
            case 'Delete':
            case 'Cancel':
            case 'Backspace':
              setSearchQuery('');
              searchInput?.current?.focus();
          }
          // if the key is a single alphanumeric character without modifier
          if (/^[a-zA-Z0-9]\b/.test(e.key) && !hasAnyModifier(e)) {
            searchInput?.current?.focus();
            setSearchQuery((old) => old + e.key);
            e.preventDefault(); // Avoid race condition to type twice in the input
          }
        }

        if (onKeyDown) onKeyDown(e);
      }}
      MenuListProps={{
        sx: {
          padding: '8px 0',
        },
      }}
      {...menuProps}
    >
      {enableSearchInput && (
        <SearchInput
          searchInput={searchInput}
          searchQuery={searchQuery}
          onChange={(e) => {
            setSearchQuery(e.currentTarget.value);
          }}
        />
      )}
      {isLoading ? (
        <MenuSkeleton
          itemCount={Math.min(options.length, 30)}
          maxHeight={maxHeight}
          disableFooter={disableFooter}
        />
      ) : (
        [
          <div
            css={{
              maxHeight: maxHeight,
              overflow: 'hidden',
              width: 300,
            }}
            tabIndex={-1}
            key="options-menu-container"
          >
            <OptionsMenu
              elements={options.map((o) => ({
                el: o,
                matches:
                  (highlightedCharactersWrapper &&
                    highlightedCharactersWrapper[o.value]) ??
                  [],
                score: 0,
              }))}
              selectedElements={new Set(tempSelectedOptions[option.value])}
              flipOption={flipOption}
            />
          </div>,
          !disableFooter && (
            <Footer
              onCancelClick={(e) => {
                if (onClose) onClose(e, 'escapeKeyDown');
                resetTempOptions();
              }}
              onApplyClick={confirmTempOptions}
              key="options-menu-footer"
            />
          ),
        ]
      )}
    </Menu>
  );
}
