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

import { DIMENSION_SEPARATOR } from '@/fleet/constants/dimension_separator';
import { OptionCategory, SelectedOptions } from '@/fleet/types';

import { StringOnlyFilterButton } from './string_only_filter_button';
import { StringOnlySelectedChip } from './string_only_selected_chip';

/**
 * Provides a simplified wrapper around `FilterButton` and `SelectedOptions` that only supports string filters
 */
export const FilterBar = ({
  filterOptions,
  selectedOptions,
  onSelectedOptionsChange,
  isLoading,
  commonOptions,
}: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  isLoading?: boolean;
  commonOptions?: string[];
}) => {
  const sortedFilterOptions = useMemo(
    () => elevateSelectedFiltersToTheTop(filterOptions, selectedOptions),
    [filterOptions, selectedOptions],
  );

  return (
    <div
      css={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}
    >
      <StringOnlyFilterButton
        filterOptions={sortedFilterOptions}
        selectedOptions={selectedOptions}
        onSelectedOptionsChange={onSelectedOptionsChange}
        isLoading={isLoading}
        commonOptions={commonOptions}
      />
      {Object.entries(selectedOptions).map(
        ([optionKey, optionValues]) =>
          optionValues?.length > 0 && (
            <StringOnlySelectedChip
              key={`selected-chip-${optionKey}`}
              filterOptions={sortedFilterOptions}
              isLoading={isLoading ?? false}
              onSelectedOptionsChange={onSelectedOptionsChange}
              selectedOptions={selectedOptions}
              optionKey={optionKey}
              optionValues={optionValues}
              dimensionSeparator={DIMENSION_SEPARATOR}
            />
          ),
      )}
    </div>
  );
};

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
