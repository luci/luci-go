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

import { useMemo } from 'react';

import { OptionCategory, SelectedOptions } from '@/fleet/types';

import { DeviceListFilterButton } from './device_list_filter_button';
import { DeviceListSelectedChip } from './device_list_selected_chip';
import { DeviceSearchBar } from './device_search_bar';

function elevateSelectedFiltersToTheTop(
  filterOptions: OptionCategory[],
  selectedOptions: SelectedOptions,
): OptionCategory[] {
  // Unselected filters are also considered for reorganizing,
  // as they are included in the selectedOptions with an empty array.
  return filterOptions.map((filter) => {
    if (filter.value in selectedOptions) {
      filter.options.sort((a, b) => {
        const aIsSelected = selectedOptions[filter.value].includes(a.value);
        const bIsSelected = selectedOptions[filter.value].includes(b.value);
        if (aIsSelected && !bIsSelected) return -1;
        if (!aIsSelected && bIsSelected) return 1;

        return a.value.localeCompare(b.value);
      });
    }

    return filter;
  });
}

export const DeviceListFilterBar = ({
  filterOptions,
  selectedOptions,
  onSelectedOptionsChange,
  isLoading,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  isLoading?: boolean;
}) => {
  const sortedFilterOptions = useMemo(
    () => elevateSelectedFiltersToTheTop(filterOptions, selectedOptions),
    [filterOptions, selectedOptions],
  );
  const deviceOptions = useMemo(() => {
    const deviceIdCategory = sortedFilterOptions.find(
      (opt) => opt.value === 'id',
    );
    return deviceIdCategory === undefined ? [] : deviceIdCategory.options;
  }, [sortedFilterOptions]);

  return (
    <div
      css={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}
    >
      <DeviceSearchBar
        options={deviceOptions}
        applySelectedOption={(optionId) =>
          onSelectedOptionsChange({ id: [optionId] })
        }
      />
      <DeviceListFilterButton
        filterOptions={sortedFilterOptions}
        selectedOptions={selectedOptions}
        onSelectedOptionsChange={onSelectedOptionsChange}
        isLoading={isLoading}
      />
      {Object.entries(selectedOptions).map(
        ([optionKey, optionValues]) =>
          optionValues?.length > 0 && (
            <DeviceListSelectedChip
              key={`selected-chip-${optionKey}`}
              filterOptions={sortedFilterOptions}
              isLoading={isLoading ?? false}
              onSelectedOptionsChange={onSelectedOptionsChange}
              selectedOptions={selectedOptions}
              optionKey={optionKey}
              optionValues={optionValues}
            />
          ),
      )}
    </div>
  );
};
