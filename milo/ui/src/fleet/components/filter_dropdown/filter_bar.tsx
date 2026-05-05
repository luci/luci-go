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

import { useRef, useState } from 'react';

import { useShortcut } from '@/fleet/components/shortcut_provider';
import { COMMON_DEVICE_FILTERS } from '@/fleet/config/device_config';
import { isTyping } from '@/fleet/utils/field_typing';

import { FilterCategory } from '../filters/use_filters';

import { FilterDropdown, FilterDropdownHandle } from './filter_dropdown';
import { SearchBar } from './search_bar';

interface FilterBarProps {
  filterCategoryDatas: FilterCategory[];
  onApply: () => void;
  isLoading?: boolean;
  searchPlaceholder?: string;
}

export function FilterBar({
  filterCategoryDatas,
  onApply,
  isLoading,
  searchPlaceholder,
}: FilterBarProps) {
  const [value, setValue] = useState('');
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);

  const searchBarRef = useRef<HTMLInputElement>(null);
  const filterDropdownRef = useRef<FilterDropdownHandle>(null);

  const onValueChange = (newValue: string) => {
    if (newValue.length > value.length) {
      setIsDropdownOpen(true);
    }
    setValue(newValue);
  };

  useShortcut('Focus search bar', '/', (e) => {
    if (!isTyping(document.activeElement)) {
      e.preventDefault();
      searchBarRef.current?.focus();
    }
  });
  return (
    <div css={{ display: 'flex', gap: 8, width: '100%', position: 'relative' }}>
      <SearchBar
        value={value}
        onChange={onValueChange}
        filterCategoryDatas={filterCategoryDatas}
        onDropdownFocus={() => {
          filterDropdownRef.current?.focus();
          setIsDropdownOpen(true);
        }}
        onDropdownFocusFirstPopperElement={() =>
          filterDropdownRef.current?.focusFirstPopperElement?.() ?? false
        }
        isDropdownOpen={isDropdownOpen}
        onChangeDropdownOpen={setIsDropdownOpen}
        isLoading={isLoading}
        ref={searchBarRef}
        onChipEditApplied={onApply}
        placeholder={searchPlaceholder}
      />
      <FilterDropdown
        ref={filterDropdownRef}
        searchQuery={value}
        onSearchQueryChange={(searchQuery) => {
          setValue(searchQuery);
        }}
        onSearchBarFocus={() => {
          searchBarRef.current?.focus();
        }}
        filterCategoryDatas={filterCategoryDatas}
        onApply={() => {
          onApply();
          setValue('');
          searchBarRef.current?.focus();
        }}
        anchorEl={isDropdownOpen ? searchBarRef.current : null}
        onClose={() => {
          setIsDropdownOpen(false);
        }}
        commonOptions={COMMON_DEVICE_FILTERS}
        isLoading={isLoading}
      />
    </div>
  );
}
