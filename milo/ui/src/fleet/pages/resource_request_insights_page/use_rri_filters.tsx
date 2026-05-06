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
import { useEffect, useMemo, useState } from 'react';

import { OptionComponent } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { DateFilterCategoryDataBuilder } from '@/fleet/components/filters/date_filter';
import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useFilters } from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FilterType } from '@/fleet/types';
import { GetResourceRequestsMultiselectFilterValuesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import type { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

import { COLUMNS } from './rri_columns';

export type DateFilterData = {
  min?: DateOnly;
  max?: DateOnly;
};

export type RangeFilterData = {
  min?: number;
  max?: number;
};

const mapToOptions = (elements: readonly string[]) =>
  Array.from(elements).map((el) => ({
    label: el === '' ? BLANK_VALUE : el,
    value: `"${el}"`,
  }));

const makeStringListFilter =
  (
    label: string,
    elements: readonly string[] | undefined,
    comparator: ':' | '=' = '=',
  ) =>
  () =>
    new StringListFilterCategoryBuilder()
      .setLabel(label)
      .setComparator(comparator)
      .setOptions(mapToOptions(elements ?? []));

const makeDateFilter = (label: string) => () =>
  new DateFilterCategoryDataBuilder().setLabel(label).setIsDateOnly(true);

const makeRangeFilter = (label: string, min: number, max: number) => () =>
  new RangeFilterCategoryBuilder().setLabel(label).setMin(min).setMax(max);

export const getFilterBuilders = (
  data: GetResourceRequestsMultiselectFilterValuesResponse | undefined,
) => ({
  rr_id: makeStringListFilter(COLUMNS.rr_id.header, data?.rrIds),
  resource_details: makeStringListFilter(
    COLUMNS.resource_details.header,
    data?.resourceDetails,
  ),
  fulfillment_status: () =>
    new StringListFilterCategoryBuilder()
      .setLabel(COLUMNS.fulfillment_status.header)
      .setOptions([
        { label: 'Not Started', value: '"NOT_STARTED"' },
        { label: 'In Progress', value: '"IN_PROGRESS"' },
        { label: 'Complete', value: '"COMPLETE"' },
      ]),
  resource_request_target_delivery_date: makeDateFilter(
    COLUMNS.resource_request_target_delivery_date.header,
  ),
  resource_request_actual_delivery_date: makeDateFilter(
    COLUMNS.resource_request_actual_delivery_date.header,
  ),
  material_sourcing_actual_delivery_date: makeDateFilter(
    COLUMNS.material_sourcing_actual_delivery_date.header,
  ),
  build_actual_delivery_date: makeDateFilter(
    COLUMNS.build_actual_delivery_date.header,
  ),
  qa_actual_delivery_date: makeDateFilter(
    COLUMNS.qa_actual_delivery_date.header,
  ),
  config_actual_delivery_date: makeDateFilter(
    COLUMNS.config_actual_delivery_date.header,
  ),
  customer: makeStringListFilter(COLUMNS.customer.header, data?.customer),
  resource_name: makeStringListFilter(
    COLUMNS.resource_name.header,
    data?.resourceName,
  ),
  accepted_quantity: makeRangeFilter(
    COLUMNS.accepted_quantity.header,
    0,
    10000,
  ),
  slippage: makeRangeFilter(COLUMNS.slippage.header, -365, 365),
  criticality: makeStringListFilter(
    COLUMNS.criticality.header,
    data?.criticality,
  ),
  request_approval: makeStringListFilter(
    COLUMNS.request_approval.header,
    data?.requestApproval,
  ),
  resource_pm: makeStringListFilter(
    COLUMNS.resource_pm.header,
    data?.resourcePm,
  ),
  fulfillment_channel: makeStringListFilter(
    COLUMNS.fulfillment_channel.header,
    data?.fulfillmentChannel,
  ),
  execution_status: makeStringListFilter(
    COLUMNS.execution_status.header,
    data?.executionStatus,
  ),
  resource_groups: makeStringListFilter(
    COLUMNS.resource_groups.header,
    data?.resourceGroups,
    ':',
  ),
  rr_bug_status: makeStringListFilter(
    COLUMNS.rr_bug_status.header,
    data?.resourceRequestBugStatus,
  ),
});

type FilterBuilders = ReturnType<typeof getFilterBuilders>;
export type RriFilterKey = keyof FilterBuilders;

type InferBuilderType<T> = T extends () => infer B ? B : never;

type RriBuildersInstance = {
  [K in keyof FilterBuilders]: InferBuilderType<FilterBuilders[K]>;
};

type MapBuilderToType<B> = B extends StringListFilterCategoryBuilder
  ? string[]
  : B extends RangeFilterCategoryBuilder
    ? RangeFilterData
    : B extends DateFilterCategoryDataBuilder
      ? DateFilterData
      : never;

export type RriFilters = {
  [K in RriFilterKey]?: MapBuilderToType<
    InferBuilderType<ReturnType<typeof getFilterBuilders>[K]>
  >;
};

export interface RriFilterOption {
  value: RriFilterKey;
  type?: FilterType;
  getChildrenSearchScore?: (searchQuery: string) => number;
  optionsComponent?: OptionComponent<ResourceRequestInsightsOptionComponentProps>;
  optionsComponentProps?: Record<string, unknown>;
}

export interface ResourceRequestInsightsOptionComponentProps {
  option: RriFilterOption;
  filters: RriFilters | undefined;
  onFiltersChange: (x: RriFilters) => void;
  onClose: () => void;
  onApply: () => void;
}

export const useRriFilters = () => {
  const client = useFleetConsoleClient();
  const query = useQuery(
    client.GetResourceRequestsMultiselectFilterValues.query({}),
  );

  const [filterOptions, setFilterOptions] = useState<
    RriBuildersInstance | undefined
  >(undefined);

  const { filterValues, parseError, aip160 } = useFilters(filterOptions, {
    areFilterValuesLoading: query.isLoading,
  });

  const nextFilterOptions = useMemo(() => {
    if (!query.data) return undefined;

    const builders = getFilterBuilders(query.data);
    return Object.fromEntries(
      Object.entries(builders).map(([key, getter]) => [key, getter()]),
    ) as RriBuildersInstance;
  }, [query.data]);

  useEffect(() => {
    setFilterOptions(nextFilterOptions);
  }, [nextFilterOptions]);

  return {
    filterValues,
    aipString: aip160(),
    isLoading: query.isLoading,
    parseError,
  };
};
