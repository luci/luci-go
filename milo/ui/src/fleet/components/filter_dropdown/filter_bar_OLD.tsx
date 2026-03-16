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

import {
  FilterCategoryData_OLD,
  FilterDropdown_OLD,
  FilterDropdownHandle_OLD,
} from './filter_dropdown_OLD';
import { SearchBar_OLD } from './search_bar_OLD';

interface FilterBarProps_OLD<T> {
  filterCategoryDatas: FilterCategoryData_OLD<T>[];
  selectedOptions: string[];
  onApply: () => void;
  getChipLabel: (option: FilterCategoryData_OLD<T>) => string;
  onChipDeleted: (option: FilterCategoryData_OLD<T>) => void;
  isLoading?: boolean;
  searchPlaceholder?: string;
}

/** @deprecated use FilterBar instead */
export function FilterBar_OLD<T>({
  filterCategoryDatas,
  selectedOptions,
  onApply,
  getChipLabel,
  onChipDeleted,
  isLoading,
  searchPlaceholder,
}: FilterBarProps_OLD<T>) {
  const [value, setValue] = useState('');
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);

  const searchBarRef = useRef<HTMLInputElement>(null);
  const filterDropdownRef = useRef<FilterDropdownHandle_OLD>(null);

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
      <SearchBar_OLD
        value={value}
        onChange={onValueChange}
        selectedOptions={
          filterCategoryDatas.filter((f) =>
            selectedOptions.includes(f.value),
          ) as FilterCategoryData_OLD<unknown>[]
        }
        onDropdownFocus={() => {
          filterDropdownRef.current?.focus();
          setIsDropdownOpen(true);
        }}
        isDropdownOpen={isDropdownOpen}
        onChangeDropdownOpen={setIsDropdownOpen}
        isLoading={isLoading}
        ref={searchBarRef}
        onChipDeleted={(option: FilterCategoryData_OLD<unknown>) => {
          onChipDeleted(option as FilterCategoryData_OLD<T>);
        }}
        getLabel={(o: FilterCategoryData_OLD<unknown>) =>
          getChipLabel(o as FilterCategoryData_OLD<T>)
        }
        onChipEditApplied={onApply}
        placeholder={searchPlaceholder}
      />
      <FilterDropdown_OLD
        ref={filterDropdownRef}
        searchQuery={value}
        onSearchQueryChange={(searchQuery) => {
          setValue(searchQuery);
        }}
        onSearchBarFocus={() => {
          searchBarRef.current?.focus();
        }}
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        filterOptions={filterCategoryDatas as any}
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
