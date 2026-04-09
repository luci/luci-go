// Copyright 2023 The LUCI Authors.
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

import { SelectedOptions } from '@/fleet/types';

import { parseFilters, stringifyFilters } from './parser/parser';

// TODO: b/404269860 extract to a common location

export const FILTERS_PARAM_KEY = 'filters';

/**
 * Get the filter parameter from the URLSearchParams.
 */
export function getFilterValue(params: URLSearchParams) {
  return params.get(FILTERS_PARAM_KEY) ?? '';
}

function rewriteLegacyKeys(filters: SelectedOptions): SelectedOptions {
  const rewritten: SelectedOptions = {};
  for (const [key, val] of Object.entries(filters)) {
    let newKey = key;
    if (key.startsWith('swarming_labels.')) {
      newKey = 'sw.' + key.substring('swarming_labels.'.length);
    } else if (key.startsWith('ufs_labels.')) {
      newKey = 'ufs.' + key.substring('ufs_labels.'.length);
    }
    rewritten[newKey] = val;
  }
  return rewritten;
}

/**
 * Get the filter from the URLSearchParams.
 */
export function getFilters(params: URLSearchParams) {
  const result = getFilterValue(params);
  const parsed = parseFilters(result);
  if (parsed.filters) {
    parsed.filters = rewriteLegacyKeys(parsed.filters);
  }
  return parsed;
}
/**
 * Update the URLSearchParams with the new filter.
 */
export function filtersUpdater(newFilters: SelectedOptions) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    searchParams.set(FILTERS_PARAM_KEY, stringifyFilters(newFilters));
    return searchParams;
  };
}

/**
 * Takes an existing URLSearchParams and appends a new filter query to it.
 * @param params Existing URL params which may include parameters other than filters.
 * @param filterName The name of the filter we're adding.
 * @param filterValue The values of the filter we're adding.
 * @returns A new URLSearchParams object with the new filters added.
 */
export function addNewFilterToParams(
  params: URLSearchParams,
  filterName: string,
  filterValue: string[],
): URLSearchParams {
  const existingFilters = getFilters(params).filters;
  const newParams = new URLSearchParams(params);
  const newFilters = stringifyFilters({
    ...existingFilters,
    [filterName]: filterValue,
  });
  newParams.set(FILTERS_PARAM_KEY, newFilters);
  return newParams;
}
