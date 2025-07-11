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

import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { OptionComponent } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { FILTERS_PARAM_KEY } from '@/fleet/components/filter_dropdown/search_param_utils/search_param_utils';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  multiselectFilterToUrlString,
  parseMultiselectFilter,
} from '@/fleet/pages/resource_request_insights_page/rri_url_utils';
import { OptionValue } from '@/fleet/types/option';
import { toIsoString } from '@/fleet/utils/dates';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';
import {
  GetResourceRequestsMultiselectFilterValuesResponse,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DateFilter } from './date_filter';
import {
  fulfillmentStatusDisplayValueMap,
  getFulfillmentStatusScoredOptions,
} from './fulfillment_status';
import { FulfillmentStatusFilter } from './fulfillment_status_filter';
import { MultiSelectFilter } from './multiselect_filter';
import { RangeFilter } from './range_filter';
import { ResourceRequestColumnKey, RRI_COLUMNS } from './rri_columns';

const FILTER_SEPARATOR = '&';

const MAX_SELECTED_CHIP_LABEL_LENGTH = 15;

export type DateFilterData = {
  min?: DateOnly;
  max?: DateOnly;
};

export type RangeFilterData = {
  min?: number;
  max?: number;
};

export const filterDescriptors = {
  rr_id: 'multi-select',
  resource_details: 'multi-select',
  fulfillment_status: 'multi-select',
  resource_request_target_delivery_date: 'date-range',
  resource_request_actual_delivery_date: 'date-range',
  material_sourcing_actual_delivery_date: 'date-range',
  build_actual_delivery_date: 'date-range',
  qa_actual_delivery_date: 'date-range',
  config_actual_delivery_date: 'date-range',
  customer: 'multi-select',
  resource_name: 'multi-select',
  accepted_quantity: 'range',
  criticality: 'multi-select',
  request_approval: 'multi-select',
  resource_pm: 'multi-select',
  fulfillment_channel: 'multi-select',
  execution_status: 'multi-select',
} as const satisfies Partial<
  Record<ResourceRequestColumnKey, 'multi-select' | 'date-range' | 'range'>
>;

export type RriFilterKey = keyof typeof filterDescriptors;

type MapDescriptorToType<T extends (typeof filterDescriptors)[RriFilterKey]> = {
  'multi-select': string[];
  'date-range': DateFilterData;
  range: RangeFilterData;
}[T];

export type RriFilters = {
  [K in RriFilterKey]?: MapDescriptorToType<(typeof filterDescriptors)[K]>;
};

export interface RriFilterOption {
  value: RriFilterKey;
  getChildrenSearchScore?: (searchQuery: string) => number;
  optionsComponent: OptionComponent<ResourceRequestInsightsOptionComponentProps>;
}

export interface ResourceRequestInsightsOptionComponentProps {
  option: RriFilterOption;
  filters: RriFilters | undefined;
  onFiltersChange: (x: RriFilters) => void;
  onClose: () => void;
  onApply: () => void;
}

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

const parseRangeFromUrl = (
  filterDict: Record<string, string>,
  key: RriFilterKey,
): RangeFilterData | undefined => {
  const min = filterDict[`${key}_min`];
  const max = filterDict[`${key}_max`];

  if (!min && !max) {
    return undefined;
  }
  return {
    min: parseInt(min),
    max: parseInt(max),
  };
};

const getFiltersFromSearchParam = (
  searchParams: URLSearchParams,
): RriFilters | undefined => {
  const paramValue = searchParams.get(FILTERS_PARAM_KEY);
  if (paramValue === null) {
    return undefined;
  }
  const rec = paramValue.split(FILTER_SEPARATOR).reduce(
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
    rr_id: parseMultiselectFilter(rec['rr_id']),
    resource_details: parseMultiselectFilter(rec['resource_details']),
    resource_request_target_delivery_date: parseDateOnlyFromUrl(
      rec,
      'resource_request_target_delivery_date',
    ),
    resource_request_actual_delivery_date: parseDateOnlyFromUrl(
      rec,
      'resource_request_actual_delivery_date',
    ),
    material_sourcing_actual_delivery_date: parseDateOnlyFromUrl(
      rec,
      'material_sourcing_actual_delivery_date',
    ),
    build_actual_delivery_date: parseDateOnlyFromUrl(
      rec,
      'build_actual_delivery_date',
    ),
    qa_actual_delivery_date: parseDateOnlyFromUrl(
      rec,
      'qa_actual_delivery_date',
    ),
    config_actual_delivery_date: parseDateOnlyFromUrl(
      rec,
      'config_actual_delivery_date',
    ),
    fulfillment_status: parseMultiselectFilter(rec['fulfillment_status']),
    customer: parseMultiselectFilter(rec['customer']),
    resource_name: parseMultiselectFilter(rec['resource_name']),
    accepted_quantity: parseRangeFromUrl(rec, 'accepted_quantity'),
    criticality: parseMultiselectFilter(rec['criticality']),
    request_approval: parseMultiselectFilter(rec['request_approval']),
    resource_pm: parseMultiselectFilter(rec['resource_pm']),
    fulfillment_channel: parseMultiselectFilter(rec['fulfillment_channel']),
    execution_status: parseMultiselectFilter(rec['execution_status']),
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
    if (type === 'multi-select') {
      const values = filters[key] as string[] | undefined;
      if (values) {
        parts.push(`${key}=${multiselectFilterToUrlString(values)}`);
      }
    }
    if (type === 'range') {
      const filter = filters[key] as RangeFilterData | undefined;
      if (filter?.min) {
        parts.push(`${key}_min=${filter.min}`);
      }
      if (filter?.max) {
        parts.push(`${key}_max=${filter.max}`);
      }
    }
  }
  return parts.join(FILTER_SEPARATOR);
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
        parts.push(`${key} >= ${toIsoString(filter.min)}`);
      }
      if (filter.max) {
        parts.push(`${key} <= ${toIsoString(filter.max)}`);
      }
    }
    if (type === 'range') {
      const filter = filters[key] as RangeFilterData | undefined;
      if (!filter) {
        continue;
      }
      if (filter.min) {
        parts.push(`${key} >= ${filter.min}`);
      }
      if (filter.max) {
        parts.push(`${key} <= ${filter.max}`);
      }
    }
    if (type === 'multi-select') {
      const values = filters[key] as string[] | undefined;
      if (!values || values.length === 0) {
        continue;
      }
      if (values) {
        parts.push(
          '(' + values.map((v) => `${key} = "${v}"`).join(' OR ') + ')',
        );
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

const mapDateFilterToSelectedChipLabel = (
  dateFilterData: DateFilterData,
): string => {
  if (!dateFilterData.min && !dateFilterData.max) {
    return '';
  }
  if (!dateFilterData.min) {
    return `before ${toIsoString(dateFilterData.max)}`;
  }
  if (!dateFilterData.max) {
    return `after ${toIsoString(dateFilterData.min)}`;
  }
  if (dateFilterData.min && dateFilterData.max) {
    return `${toIsoString(dateFilterData.min)} - ${toIsoString(dateFilterData.max)}`;
  }
  return '';
};

export const getSortedMultiselectElements = (
  data: GetResourceRequestsMultiselectFilterValuesResponse,
  option: RriFilterKey,
  searchQuery: string,
) => {
  const els = getElements(data, option).map(
    (el): OptionValue => ({ label: el, value: el }),
  );
  return fuzzySort(searchQuery)(els, (x) => x.label);
};

const getElements = (
  data: GetResourceRequestsMultiselectFilterValuesResponse,
  option: RriFilterKey,
) => {
  const map = {
    rr_id: data.rrIds,
    resource_details: data.resourceDetails,
    customer: data.customer,
    resource_name: data.resourceName,
    criticality: data.criticality,
    request_approval: data.requestApproval,
    resource_pm: data.resourcePm,
    fulfillment_channel: data.fulfillmentChannel,
    execution_status: data.executionStatus,
  } as Record<RriFilterKey, string[]>;

  return map[option];
};

export const useRriFilters = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const client = useFleetConsoleClient();
  const query = useQuery(
    client.GetResourceRequestsMultiselectFilterValues.query({}),
  );

  const filterComponents = [
    {
      value: 'rr_id',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(query.data, 'rr_id', searchQuery)[0]
              .score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'resource_details',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'resource_details',
              searchQuery,
            )[0].score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'fulfillment_status',
      getChildrenSearchScore: (searchQuery: string) =>
        getFulfillmentStatusScoredOptions(searchQuery)[0].score,
      optionsComponent: FulfillmentStatusFilter,
    },
    {
      value: 'resource_request_target_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'resource_request_actual_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'material_sourcing_actual_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'build_actual_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'qa_actual_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'config_actual_delivery_date',
      optionsComponent: DateFilter,
    },
    {
      value: 'customer',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(query.data, 'customer', searchQuery)[0]
              ?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'resource_name',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'resource_name',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'accepted_quantity',
      getChildrenSearchScore: () => 0,
      optionsComponent: RangeFilter,
    },
    {
      value: 'criticality',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'criticality',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'request_approval',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'request_approval',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'resource_pm',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'resource_pm',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'fulfillment_channel',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'fulfillment_channel',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
    {
      value: 'execution_status',
      getChildrenSearchScore: (searchQuery: string) =>
        query.data
          ? getSortedMultiselectElements(
              query.data,
              'execution_status',
              searchQuery,
            )[0]?.score
          : 0,
      optionsComponent: MultiSelectFilter,
    },
  ] as RriFilterOption[];

  const filterData = useMemo(
    () => getFiltersFromSearchParam(searchParams),
    [searchParams],
  );

  const setFilters = (newFilters: RriFilters | undefined) => {
    setSearchParams(filtersUpdater(newFilters));
  };

  const aipString = filterData ? filtersToAip(filterData) : '';

  const selectedFilterLabelMap = {
    rr_id: (v) => (v as string[]).join(', '),
    resource_details: (v) => (v as string[]).join(', '),
    fulfillment_status: (v) => {
      const values = v as (keyof typeof ResourceRequest_Status)[];
      return values
        .map((value) => fulfillmentStatusDisplayValueMap[value])
        .join(', ');
    },
    resource_request_target_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    resource_request_actual_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    material_sourcing_actual_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    build_actual_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    qa_actual_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    config_actual_delivery_date: (v) =>
      mapDateFilterToSelectedChipLabel(v as DateFilterData),
    customer: (v) => (v as string[]).join(', '),
    resource_name: (v) => (v as string[]).join(', '),
    accepted_quantity: (v) => {
      const val = v as RangeFilterData;
      if (val.min && val.max) {
        return val.min + ' - ' + val.max;
      }
      if (val.min) {
        return '> ' + val.min;
      }
      if (val.max) {
        return '< ' + val.max;
      }
      return '';
    },
    criticality: (v) => (v as string[]).join(', '),
    request_approval: (v) => (v as string[]).join(', '),
    resource_pm: (v) => (v as string[]).join(', '),
    fulfillment_channel: (v) => (v as string[]).join(', '),
    execution_status: (v) => (v as string[]).join(', '),
  } as const satisfies Record<
    RriFilterKey,
    (filterValue: RriFilters[RriFilterKey]) => string
  >;

  const getSelectedFilterLabel = (
    filterKey: RriFilterKey,
    filterValue: RriFilters[RriFilterKey],
  ): string => {
    let label: string =
      RRI_COLUMNS.find((c) => c.id === filterKey)?.gridColDef.headerName ??
      filterKey;

    if (label && label.length > MAX_SELECTED_CHIP_LABEL_LENGTH) {
      label = label?.slice(0, MAX_SELECTED_CHIP_LABEL_LENGTH);
      label += '...';
    }

    return `${label}: ${selectedFilterLabelMap[filterKey](filterValue)}`;
  };

  return {
    filterComponents,
    filterData,
    aipString,
    setFilters,
    /**
     * Make sure the value is the correct type given the key
     */
    getSelectedFilterLabel,
  };
};
