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

import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionCategory, StringListCategory } from '@/fleet/types';
import {
  GetBrowserDeviceDimensionsResponse,
  LabelValues,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const mapDimensionsToCategories = (
  dimensions: { [key: string]: LabelValues },
  columns: Record<string, { headerName?: string; filterByField?: string }>,
  getColumnId: (key: string) => string,
): OptionCategory[] => {
  return Object.entries(dimensions).map(([filterKey, filterValues]) => {
    const key = getColumnId(filterKey);
    return {
      label: columns[key]?.headerName || key,
      value: columns[key]?.filterByField || key,
      options: [
        { label: BLANK_VALUE, value: BLANK_VALUE },
        ...filterValues.values.map((value) => ({ label: value, value })),
      ],
      type: 'string_list',
    };
  });
};

export const dimensionsToFilterOptions = (
  response: GetBrowserDeviceDimensionsResponse,
  columns: Record<string, { headerName?: string; filterByField?: string }>,
): OptionCategory[] => {
  return [
    ...mapDimensionsToCategories(response.baseDimensions, columns, (k) => k),
    ...mapDimensionsToCategories(
      response.swarmingLabels,
      columns,
      (k) => `${BROWSER_SWARMING_SOURCE}.${k}`,
    ),
    ...mapDimensionsToCategories(
      response.ufsLabels,
      columns,
      (k) => `${BROWSER_UFS_SOURCE}.${k}`,
    ),
  ]
    .sort((a, b) => a.label.localeCompare(b.label)) // Sort alphabetically
    .filter((o) => (o as StringListCategory).options?.length > 0);
};
