// Copyright 2026 The LUCI Authors.
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

import _ from 'lodash';

import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';

/**
 * Removes one level of surrounding quotes if present.
 */
export const stripQuotes = (val: string): string =>
  val.startsWith('"') && val.endsWith('"') ? val.slice(1, -1) : val;

/**
 * Synchronizes a single filter category with the new filters received from MRT column filters.
 */
export const syncFilterCategory = (
  key: string,
  category: FilterCategory,
  newFilters: Record<string, string[]>,
  prevTableFilterKeys: string[],
): boolean => {
  const matchKey = _.snakeCase(normalizeFilterKey(key));

  const isInNewFilters =
    newFilters[matchKey] !== undefined || newFilters[key] !== undefined;
  const wasInTable =
    prevTableFilterKeys.includes(matchKey) || prevTableFilterKeys.includes(key);

  if (!isInNewFilters && !wasInTable) {
    return false;
  }

  const newValues = newFilters[matchKey] || newFilters[key] || [];

  let currentSelected: string[] = [];
  if (category instanceof StringListFilterCategory) {
    currentSelected = category.getSelectedOptions();
  }

  const normalizedCurrent = currentSelected.map((v) => stripQuotes(v));
  const normalizedNew = newValues.map((v) => stripQuotes(v));
  const isChanged = _.xor(normalizedCurrent, normalizedNew).length > 0;

  if (isChanged) {
    if (category instanceof StringListFilterCategory) {
      category.setSelectedOptions(newValues, true);
    }
  }
  return isChanged;
};
