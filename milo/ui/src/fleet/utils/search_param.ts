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

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';

import { OrderBy, OrderByDirection } from '../hooks/order_by';

/**
 * Takes an existing set of URLSearchParams and updates that object with a
 * new set of values.
 *
 * @param searchParams existing URLParams
 * @param key query parameter name
 * @param value query parameter value
 * @returns new URLSearchParams instance
 */
export const addOrUpdateQueryParam = (
  searchParams: URLSearchParams,
  key: string,
  value: string,
) => {
  const newParams = new URLSearchParams(searchParams.toString());
  newParams.set(key, value);
  return newParams;
};

export function getVisibilityModel(
  allColumns: string[],
  visibleColumns: string[],
): Record<string, boolean> {
  return allColumns.reduce(
    (acc, val) => ({
      ...acc,
      [val]: visibleColumns.includes(val),
    }),
    {},
  );
}

export function parseOrderByParam(orderByParam: string): OrderBy | null {
  const [field, direction] = orderByParam.split(' ');

  if (!field) {
    return null;
  }

  return {
    field: field,
    direction:
      direction === OrderByDirection.DESC
        ? OrderByDirection.DESC
        : OrderByDirection.ASC,
  };
}

export function escapeAipValue(val: string): string {
  return val.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
}

export function quoteAipKey(key: string): string {
  if (key.startsWith('"') && key.endsWith('"')) return key;
  if (/^[a-zA-Z0-9_]+$/.test(key)) return key;
  if (key.includes('.')) {
    const [prefix, ...rest] = key.split('.');
    const sub = rest.join('.').replace(/"/g, '');
    return `${prefix}."${sub}"`;
  }
  return `"${key}"`;
}

export function formatAipClause(name: string, values: string[]): string {
  if (!values?.length) return '';
  const qName = quoteAipKey(name);
  if (values.length === 1) return `${qName} = "${escapeAipValue(values[0])}"`;
  return `(${values.map((v) => `${qName} = "${escapeAipValue(v)}"`).join(' OR ')})`;
}

export function combineAipFilters(base: string, clause: string): string {
  if (!base) return clause;
  if (!clause) return base;
  return `${base} AND ${clause}`;
}

export function getLegacyFilterOrQuery(
  searchParams: URLSearchParams | undefined,
): string {
  if (!searchParams) return '';
  const raw =
    searchParams.get(FILTERS_PARAM_KEY) ||
    searchParams.get('filter') ||
    searchParams.get('q') ||
    searchParams.get('f') ||
    searchParams.get('search') ||
    '';
  if (!raw) return '';
  if (!raw.includes('=') && !raw.includes(':') && !raw.includes('(')) {
    return `id = "${escapeAipValue(raw)}"`;
  }
  return raw;
}

export function getFilterQueryString(
  filters: Record<string, string[]>,
  searchParams: URLSearchParams | undefined,
  pagerContext?: PagerContext,
): string {
  let newSearchParams = new URLSearchParams(searchParams);
  if (pagerContext) {
    newSearchParams = emptyPageTokenUpdater(pagerContext)(newSearchParams);
  }
  let currentFilters = newSearchParams.get(FILTERS_PARAM_KEY) ?? '';
  for (const [name, values] of Object.entries(filters)) {
    const clause = formatAipClause(name, values);
    currentFilters = combineAipFilters(currentFilters, clause);
  }
  if (currentFilters) {
    newSearchParams.set(FILTERS_PARAM_KEY, currentFilters);
  } else {
    newSearchParams.delete(FILTERS_PARAM_KEY);
  }
  return '?' + newSearchParams.toString();
}
