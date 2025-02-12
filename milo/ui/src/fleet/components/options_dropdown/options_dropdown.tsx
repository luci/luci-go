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

import { Menu, MenuProps, PopoverOrigin } from '@mui/material';
import { useMemo, useRef, useState } from 'react';

import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import { hasAnyModifier, keyboardUpDownHandler } from '../../utils';
import { OptionsMenu } from '../multi_select_filter/options_menu';
import { SearchInput } from '../search_input';

import { Footer } from './footer';

type OptionsDropdownProps = MenuProps & {
  onClose?: (event: object, reason: 'backdropClick' | 'escapeKeyDown') => void;
  anchorEl: HTMLElement | null;
  open: boolean;
  option: OptionCategory;
  selectedOptions: SelectedOptions;
  setSelectedOptions?: React.Dispatch<React.SetStateAction<SelectedOptions>>;
  anchorOrigin?: PopoverOrigin | undefined;
  highlightedCharacters?: Record<string, number[]>;
  disableFooter?: boolean;
  enableSearchInput?: boolean;
  onFlipOption?: (value: string) => void;
  maxHeight?: number;
};

export function OptionsDropdown({
  onClose,
  anchorEl,
  open,
  option,
  selectedOptions,
  setSelectedOptions,
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
  ...menuProps
}: OptionsDropdownProps) {
  const [tempSelectedOptions, setTempSelectedOptions] =
    useState(selectedOptions);

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
    if (setSelectedOptions) {
      setSelectedOptions(tempSelectedOptions);
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
      <div
        css={{
          maxHeight: maxHeight,
          overflow: 'hidden',
          width: 300,
        }}
        tabIndex={-1}
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
      </div>
      {!disableFooter && (
        <Footer
          onCancelClick={(e) => {
            if (onClose) onClose(e, 'escapeKeyDown');
            resetTempOptions();
          }}
          onApplyClick={confirmTempOptions}
        />
      )}
    </Menu>
  );
}
