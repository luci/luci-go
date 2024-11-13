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

import { SelectedFilters } from '../types';

const FILTERS_PARAM_KEY = 'filters';

/** The input is expected to follow AIP - 160.
 * For now it's limited to inputs following the format:
 *   "nameSpace.key1 = (value1 AND value2) nameSpace2.key = value".
 * Nested parentheses are not supported.
 * E.g.: "fleet_labels.pool = (default AND test)"
 * TODO: Consider moving this to a shared location
 */
export const parseFilters = (
  str: string,
  filters: SelectedFilters = {},
): SelectedFilters => {
  const firstEqIdx = str.indexOf('=');
  if (firstEqIdx === -1) return filters;

  const lhs = str.substring(0, firstEqIdx).trim();
  const [nameSpace, key] = lhs.split('.');
  if (nameSpace === undefined || key === undefined) {
    throw Error('Values are expected to have the form nameSpace.key');
  }

  const rest = str.substring(firstEqIdx + 1).trim();

  let values: string[] = [];
  let rhsEndIdx = -1;
  if (rest[0] === '(') {
    rhsEndIdx = rest.indexOf(')', 1);
    if (rhsEndIdx === -1) {
      throw Error('Missing closing parenthesis');
    }

    values = rest
      .substring(1, rhsEndIdx)
      .split('AND')
      .map((s) => s.trim());
    if (values.some((v) => v === '')) {
      throw Error('Found a hanging ANDs');
    }
  } else {
    rhsEndIdx = rest.match(/\s/)?.index ?? rest.length;
    values = [rest.substring(0, rhsEndIdx)];
  }

  return parseFilters(rest.substring(rhsEndIdx + 1), {
    ...filters,
    [nameSpace]: {
      ...filters[nameSpace],
      [key]: [...(filters[nameSpace]?.[key] ?? []), ...values],
    },
  });
};

/**
 * The output is expected to follow AIP - 160.
 * For now it's limited to outputs following the format:
 *   "nameSpace.key1 = (value1 AND value2) nameSpace2.key = value".
 * E.g.: "fleet_labels.pool = (default AND test)"
 * TODO: Consider moving this to a shared location
 */
export const stringifyFilters = (filters: SelectedFilters): string =>
  Object.entries(filters)
    .map(([nameSpace, entries]) =>
      Object.entries(entries ?? {})
        .filter(([_key, values]) => values && values[0])
        .map(([key, values]) =>
          values.length > 1
            ? `${nameSpace}.${key} = (${values.join(' AND ')})`
            : `${nameSpace}.${key} = ${values[0]}`,
        )
        .join(' '),
    )
    .join(' ');

/**
 * Get the filter from the URLSearchParams.
 */
export function getFilters(params: URLSearchParams) {
  const status = params.get(FILTERS_PARAM_KEY) ?? '';
  try {
    return parseFilters(status);
  } catch {
    return {};
  }
}

/**
 * Update the URLSearchParams with the new filter.
 */
export function filtersUpdater(newFilters: SelectedFilters) {
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
