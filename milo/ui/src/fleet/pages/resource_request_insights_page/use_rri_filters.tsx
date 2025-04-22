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

import { ResourceRequestColumnKey } from './resource_request_insights_page';

const FILTERS_PARAM_KEY = 'filters';

export type DateFilterData = {
  min?: DateOnly;
  max?: DateOnly;
};

export const filterDescriptors = {
  rr_id: 'string',
  material_sourcing_target_delivery_date: 'date-range',
  build_target_delivery_date: 'date-range',
  qa_target_delivery_date: 'date-range',
  config_target_delivery_date: 'date-range',
} as const satisfies Partial<
  Record<ResourceRequestColumnKey, 'string' | 'date-range'>
>;

export type RriFilterKey = keyof typeof filterDescriptors;

type MapDescriptorToType<T extends (typeof filterDescriptors)[RriFilterKey]> = {
  string: string;
  'string-array': string[];
  'date-range': DateFilterData;
}[T];

export type RriFilters = {
  [K in RriFilterKey]?: MapDescriptorToType<(typeof filterDescriptors)[K]>;
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

const parseDateOnlyFromUrl = (
  filterDict: Record<string, string>,
  key: RriFilterKey,
): DateFilterData | undefined => {
  const min = parseDateOnly(filterDict[`${key}_min`]);
  const max = parseDateOnly(filterDict[`${key}_max`]);

  if (!min && !max) {
    return undefined;
  }
  return {
    min: min,
    max: max,
  };
};

const getFiltersFromSearchParam = (
  searchParams: URLSearchParams,
): RriFilters | undefined => {
  const paramValue = searchParams.get(FILTERS_PARAM_KEY);
  if (paramValue === null) {
    return undefined;
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

  return {
    rr_id: undefined, // TODO: apply when rr_id filter is implemented
    material_sourcing_target_delivery_date: parseDateOnlyFromUrl(
      rec,
      'material_sourcing_target_delivery_date',
    ),
    build_target_delivery_date: parseDateOnlyFromUrl(
      rec,
      'build_target_delivery_date',
    ),
    qa_target_delivery_date: parseDateOnlyFromUrl(
      rec,
      'qa_target_delivery_date',
    ),
    config_target_delivery_date: parseDateOnlyFromUrl(
      rec,
      'config_target_delivery_date',
    ),
  } satisfies Record<RriFilterKey, unknown>;
};

const filtersToUrlString = (filters: RriFilters): string => {
  const parts: string[] = [];
  for (const key of Object.keys(filterDescriptors) as RriFilterKey[]) {
    if (!(key in filters)) {
      continue;
    }

    const type = filterDescriptors[key];
    if (type === 'date-range') {
      const filter = filters[key] as DateFilterData | undefined;
      if (filter?.min) {
        parts.push(`${key}_min=${toIsoString(filter.min)}`);
      }
      if (filter?.max) {
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

const filtersToAip = (filters: RriFilters): string => {
  const parts: string[] = [];
  for (const key of Object.keys(filterDescriptors) as RriFilterKey[]) {
    if (!(key in filters)) {
      continue;
    }

    const type = filterDescriptors[key];

    if (type === 'date-range') {
      const filter = filters[key] as DateFilterData | undefined;
      if (!filter) {
        continue;
      }
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
      const filter = filters[key] as string | undefined;
      if (!filter) {
        continue;
      }
      if (filter) {
        parts.push(`${key} = ${filter}`);
      }
    }
  }
  return parts.join(' AND ');
};

function filtersUpdater(newFilters: RriFilters | undefined) {
  return (params: URLSearchParams) => {
    const searchParams = new URLSearchParams(params);
    if (
      !newFilters ||
      Object.values(newFilters).filter((x) => x).length === 0
    ) {
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

  const setFilters = (newFilters: RriFilters | undefined) => {
    setSearchParams(filtersUpdater(newFilters));
  };

  const aipString = filters ? filtersToAip(filters) : '';

  return [filters, aipString, setFilters] as const;
};
