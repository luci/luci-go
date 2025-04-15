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

import { toIsoString } from '@/fleet/utils/dates';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

const FILTERS_PARAM_KEY = 'filters';

type DateFilter = {
  min?: DateOnly;
  max?: DateOnly;
};

const filterDescriptors = {
  rr_id: 'string',
  material_sourcing_target_delivery_date: 'date-range',
} as const;

type FilterKey = keyof typeof filterDescriptors;

type MapDescriptorToType<T extends string> = T extends 'string'
  ? string
  : T extends 'date-range'
    ? DateFilter
    : never;

export type Filters = {
  [K in FilterKey]?: MapDescriptorToType<(typeof filterDescriptors)[K]>;
};

const parseDateOnly = (
  param: string | null | undefined,
): DateOnly | undefined => {
  if (!param) {
    return undefined;
  }

  return {
    year: parseInt(param.slice(0, 4)),
    month: parseInt(param.slice(5, 7)),
    day: parseInt(param.slice(8, 10)),
  } as DateOnly;
};

const getFiltersFromSearchParam = (searchParams: URLSearchParams): Filters => {
  const paramValue = searchParams.get(FILTERS_PARAM_KEY);
  if (paramValue === null) {
    return { material_sourcing_target_delivery_date: {} };
  }
  const rec = paramValue.split(' ').reduce(
    (acc, part) => {
      const kv = part
        .trim()
        .split('=')
        .map((v) => v.trim());
      acc[kv[0]] = kv[1];
      return acc;
    },
    {} as Record<string, string>,
  );

  const filters: Filters = {
    rr_id: rec.rr_id,
    material_sourcing_target_delivery_date: {
      min:
        parseDateOnly(rec.material_sourcing_target_delivery_date_min) ??
        undefined,
      max:
        parseDateOnly(rec.material_sourcing_target_delivery_date_max) ??
        undefined,
    },
  };
  return filters;
};

const filtersToUrlString = (filters: Filters): string => {
  const parts: string[] = [];
  for (const key of Object.keys(filterDescriptors) as FilterKey[]) {
    if (!(key in filters)) {
      continue;
    }

    const type = filterDescriptors[key];
    if (type === 'date-range') {
      const filter = filters[key] as DateFilter;
      if (filter.min) {
        parts.push(`${key}_min=${toIsoString(filter.min)}`);
      }
      if (filter.max) {
        parts.push(`${key}_max=${toIsoString(filter.max)}`);
      }
    }
    if (type === 'string') {
      const filter = filters[key] as string;
      if (filter) {
        parts.push(`${key} = ${filter}`);
      }
    }
  }
  return parts.join(' ');
};

const filtersToAip = (filters: Filters): string => {
  const parts: string[] = [];
  for (const key of Object.keys(filterDescriptors) as FilterKey[]) {
    if (!(key in filters)) {
      continue;
    }

    const type = filterDescriptors[key];

    if (type === 'date-range') {
      const filter = filters[key] as DateFilter;
      if (filter.min) {
        parts.push(
          `${key} > ${toIsoString(filter.min)}`, // TODO: replace with GTE
        );
      }
      if (filter.max) {
        parts.push(
          `${key} < ${toIsoString(filter.max)}`, // TODO: replace with LTE
        );
      }
    }
    if (type === 'string') {
      const filter = filters[key] as string;
      if (filter) {
        parts.push(`${key} = ${filter}`);
      }
    }
  }
  return parts.join(' AND ');
};

function filtersUpdater(newFilters: Filters) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (Object.keys(newFilters).length === 0) {
      searchParams.delete(FILTERS_PARAM_KEY);
    } else {
      searchParams.set(FILTERS_PARAM_KEY, filtersToUrlString(newFilters));
    }
    return searchParams;
  };
}

export const useRriFilters = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const filters = useMemo(
    () => getFiltersFromSearchParam(searchParams),
    [searchParams],
  );

  const setFilters = (newSelectedOptions: Filters) => {
    setSearchParams(filtersUpdater(newSelectedOptions));
  };

  const aipString = filtersToAip(filters);

  return [filters, aipString, setFilters] as const;
};
