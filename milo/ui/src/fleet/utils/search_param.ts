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

import { GridColumnVisibilityModel } from '@mui/x-data-grid';

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';

import { addNewFilterToParams } from '../components/filter_dropdown/search_param_utils';
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
): GridColumnVisibilityModel {
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

export function getFilterQueryString(
  filters: Record<string, string[]>,
  searchParams: URLSearchParams,
  pagerContext?: PagerContext,
): string {
  let newSearchParams = new URLSearchParams(searchParams);
  if (pagerContext) {
    newSearchParams = emptyPageTokenUpdater(pagerContext)(newSearchParams);
  }
  for (const [name, values] of Object.entries(filters)) {
    newSearchParams = addNewFilterToParams(newSearchParams, name, values);
  }
  return '?' + newSearchParams.toString();
}
