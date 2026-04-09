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

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionCategory, StringListCategory } from '@/fleet/types';
import {
  GetDeviceDimensionsResponse,
  LabelValues,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

const mapDimensionsToCategories = (
  dimensions: { [key: string]: LabelValues },
  columns: Record<string, { header?: string; filterByField?: string }>,
  getColumnId: (key: string) => string,
  isBase: boolean,
): OptionCategory[] => {
  return Object.entries(dimensions).map(([filterKey, filterValues]) => {
    const key = getColumnId(filterKey);
    const col = columns[key];
    const label = (
      typeof col?.header === 'string' ? col.header : key
    ) as string;
    const value = col?.filterByField || (isBase ? key : `labels."${key}"`);
    return {
      label,
      value,
      options: [
        { label: BLANK_VALUE, value: BLANK_VALUE },
        ...filterValues.values.map((value) => ({ label: value, value })),
      ],
      type: 'string_list',
    };
  });
};

export const dimensionsToFilterOptions = (
  response: GetDeviceDimensionsResponse,
  columns: Record<string, { header?: string; filterByField?: string }>,
): OptionCategory[] => {
  return [
    ...mapDimensionsToCategories(
      response.baseDimensions,
      columns,
      (k) => k,
      true,
    ),
    ...mapDimensionsToCategories(response.labels, columns, (k) => k, false),
  ]
    .sort((a, b) => a.label.localeCompare(b.label)) // Sort alphabetically
    .filter((o) => (o as StringListCategory).options?.length > 0);
};
