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

import { COMMON_DEVICE_FILTERS } from '@/fleet/config/device_config';

import {
  FilterCategoryData,
  FilterDropdown,
  FilterDropdownHandle,
} from './filter_dropdown';
import { SearchBar } from './search_bar';

interface FilterBarProps<T> {
  filterCategoryDatas: FilterCategoryData<T>[];
  selectedOptions: string[];
  onApply: () => void;
  getChipLabel: (option: FilterCategoryData<T>) => string;
  onChipDeleted: (option: FilterCategoryData<T>) => void;
  isLoading?: boolean;
}

export function FilterBar<T>({
  filterCategoryDatas,
  selectedOptions,
  onApply,
  getChipLabel,
  onChipDeleted,
  isLoading,
}: FilterBarProps<T>) {
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

  return (
    <div css={{ display: 'flex', gap: 8, width: '100%', position: 'relative' }}>
      <SearchBar
        value={value}
        onChange={onValueChange}
        selectedOptions={
          filterCategoryDatas.filter((f) =>
            selectedOptions.includes(f.value),
          ) as FilterCategoryData<unknown>[]
        }
        onDropdownFocus={() => {
          filterDropdownRef.current?.focus();
          setIsDropdownOpen(true);
        }}
        onChangeDropdownOpen={setIsDropdownOpen}
        isLoading={isLoading}
        ref={searchBarRef}
        onChipDeleted={(option) => {
          onChipDeleted(option as FilterCategoryData<T>);
        }}
        getLabel={(o) => getChipLabel(o as FilterCategoryData<T>)}
        onChipEditApplied={onApply}
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
