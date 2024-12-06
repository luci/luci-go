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

import { AddFilterButton } from './add_filter_button';
import { SelectedChip } from './selected_chip';
import { FilterOption, SelectedFilters } from './types';

export const MultiSelectFilter = ({
  filterOptions,
  selectedOptions,
  setSelectedOptions,
}: {
  filterOptions: FilterOption[];
  selectedOptions: SelectedFilters;
  setSelectedOptions: React.Dispatch<React.SetStateAction<SelectedFilters>>;
}) => {
  return (
    <div style={{ display: 'flex', gap: 5, flexWrap: 'wrap' }}>
      {/*

        Object.keys(selectedOptions).filter((nameSpace) =>
        Object.values(selectedOptions[nameSpace]).some((x) => x.length !== 0)
      ).map(nameSpace => Object.entries(selectedOptions[nameSpace]).map([optionKey, optionValue]) => (
      <SelectedChip
        option={option}
        key={`selected-chip-${idx}`}
        filterOptions={filterOptions}
        selectedOptions={selectedOptions}
        setSelectedOptions={setSelectedOptions}
      />

      ))
      */}

      {filterOptions.map(
        (option, idx) =>
          option.options?.some((o2) =>
            selectedOptions[option.value]?.includes(o2.value),
          ) && (
            <SelectedChip
              option={option}
              key={`selected-chip-${idx}`}
              selectedOptions={selectedOptions}
              setSelectedOptions={setSelectedOptions}
            />
          ),
      )}
      <AddFilterButton
        filterOptions={filterOptions}
        selectedOptions={selectedOptions}
        setSelectedOptions={setSelectedOptions}
      />
    </div>
  );
};
