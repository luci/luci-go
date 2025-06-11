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

// TODO: b/404269860 extract to a common location

export const FILTERS_PARAM_KEY = 'filters';

export type GetFiltersResult =
  | { filters: SelectedOptions; error: undefined }
  | { filters: undefined; error: Error };

/** The input is expected to follow AIP - 160.
 * For now it's limited to inputs following the format:
 *   'key1 = ("value1" OR "value2") key = "value"'.
 * Nested parentheses are not supported.
 * E.g.: 'fleet_labels.pool = ("default" OR "test")'
 * TODO: Consider moving this to a shared location
 */
export const parseFilters = (
  str: string,
  filters: SelectedOptions = {},
): GetFiltersResult => {
  const firstEqIdx = str.indexOf('=');
  if (firstEqIdx === -1) return { filters, error: undefined };

  const key = str.substring(0, firstEqIdx).trim();

  const rest = str.substring(firstEqIdx + 1).trim();

  let values: string[] = [];
  let rhsEndIdx = -1;
  if (rest[0] === '(') {
    rhsEndIdx = rest.indexOf(')', 1);
    if (rhsEndIdx === -1) {
      return {
        filters: undefined,
        error: Error('Missing closing parenthesis'),
      };
    }

    values = rest
      .substring(1, rhsEndIdx)
      .split(/\s+OR\s+(?=(?:[^"]*(?:"(?:\\.|[^"])*"[^"]*)*)*$)/) // Find the word OR, peek ahead and check if there are an even number of quotes (skip escaped quotes).
      .map((s) => s.trim().replace(/^"/, '').replace(/"$/, ''));
    if (values.some((v) => v === '')) {
      return { filters: undefined, error: Error('Found a hanging ORs') };
    }
  } else if (rest[0] === '"') {
    rhsEndIdx = rest.indexOf('"', 1) ?? rest.length;
    values = [rest.substring(1, rhsEndIdx)];
  } else {
    return {
      filters: undefined,
      error: Error(`Unexpected character '${rest[0]}': should be one of '("'`),
    };
  }

  return parseFilters(rest.substring(rhsEndIdx + 1), {
    ...filters,
    [key]: [...(filters[key] ?? []), ...values],
  });
};

/**
 * The output is expected to follow AIP - 160.
 * For now it's limited to outputs following the format:
 *   "key1 = (value1 OR value2) key = value".
 * E.g.: "fleet_labels.pool = (default OR test)"
 * It also encloses values in quotes, as values can contain whitespaces,
 * and AIP-160 treats them as a whole.
 * More information: see the STRING description:
 * https://google.aip.dev/assets/misc/ebnf-filtering.txt
 * TODO: Consider moving this to a shared location
 */
export const stringifyFilters = (filters: SelectedOptions): string =>
  Object.entries(filters)
    .filter(([_key, values]) => values && values[0])
    .map(([key, values]) =>
      values.length > 1
        ? `${key} = (${values.map((v) => `"${v}"`).join(' OR ')})`
        : `${key} = "${values[0]}"`,
    )
    .join(' ');

/**
 * Get the filter parameter from the URLSearchParams.
 */
export function getFilterValue(params: URLSearchParams) {
  return params.get(FILTERS_PARAM_KEY) ?? '';
}

/**
 * Get the filter from the URLSearchParams.
 */
export function getFilters(params: URLSearchParams) {
  const result = getFilterValue(params);
  return parseFilters(result);
}

/**
 * Update the URLSearchParams with the new filter.
 */
export function filtersUpdater(newFilters: SelectedOptions) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (Object.keys(newFilters).length === 0) {
      searchParams.delete(FILTERS_PARAM_KEY);
    } else {
      searchParams.set(FILTERS_PARAM_KEY, stringifyFilters(newFilters));
    }
    return searchParams;
  };
}

/**
 * Takes an existing URLSearchParams and appends a new filter query to it.
 * @param params Existing URL params which may include parameters other than filters
 * @param filterName The name of the filter we're adding
 * @param filterValue The values of the filter we're adding
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
