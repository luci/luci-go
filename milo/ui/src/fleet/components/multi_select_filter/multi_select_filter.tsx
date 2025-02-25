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

import { OptionCategory } from '@/fleet/types';
import { SelectedOptions } from '@/fleet/types';

import { AddFilterButton } from './add_filter_button';
import { SelectedChip } from './selected_chip';

export const MultiSelectFilter = ({
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
  return (
    <div css={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      {filterOptions.map(
        (option, idx) =>
          option.options?.some((o2) =>
            selectedOptions[option.value]?.includes(o2.value),
          ) && (
            <SelectedChip
              option={option}
              key={`selected-chip-${idx}`}
              selectedOptions={selectedOptions}
              onSelectedOptionsChange={onSelectedOptionsChange}
              isLoading={isLoading}
            />
          ),
      )}
      <AddFilterButton
        filterOptions={filterOptions}
        selectedOptions={selectedOptions}
        onSelectedOptionsChange={onSelectedOptionsChange}
        isLoading={isLoading}
      />
    </div>
  );
};
