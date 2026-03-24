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
import {
  OptionCategory,
  SelectedOptions,
  StringListCategory,
} from '@/fleet/types';

import { FilterBar } from '../filter_dropdown/filter_bar';
import { StringListFilterCategoryBuilder } from '../filters/string_list_filter';
import { useFilters } from '../filters/use_filters';

export function DeviceListFilterBar({
  filterOptions,
  isLoading,
  searchPlaceholder,
}: {
  filterOptions: OptionCategory[];
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  isLoading?: boolean;
  searchPlaceholder?: string;
}) {
  const filterCategoryBuilders = Object.fromEntries(
    filterOptions.map((option) => {
      if (option.type === 'date')
        return [
          option.value,
          new StringListFilterCategoryBuilder()
            .setLabel(`UNSUPPORTED_TYPE ${option.type} - ${option.label}`)
            .setOptions([]),
        ] as const;

      if (option.type === 'string_list' || option.type === undefined)
        return [
          option.value,
          new StringListFilterCategoryBuilder()
            .setLabel(option.label)
            .setOptions(
              ((option as StringListCategory).options || []).map((o) => ({
                label: o.label,
                key: o.value,
              })),
            ),
        ] as const;

      throw new Error();
    }),
  );

  const { filterValues } = useFilters(filterCategoryBuilders);

  return (
    <FilterBar
      filterCategoryDatas={Object.values(filterValues ?? {}).filter(
        (f) => f !== undefined,
      )}
      onApply={() => {}}
      isLoading={isLoading}
      searchPlaceholder={searchPlaceholder}
    />
  );
}
