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

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const getLabelFromOverride = (
  key: string,
  labelsOverride: Record<
    string,
    { header?: string | unknown; headerName?: string }
  >,
): string | undefined => {
  const override = labelsOverride?.[key];
  if (!override) return undefined;
  return typeof override.header === 'string'
    ? override.header
    : override.headerName;
};

/**
 * Converts a response from GetDeviceDimensions into a list of options
 * for <FilterBar />
 */
export const dimensionsToFilterOptions = (
  response: GetDeviceDimensionsResponse,
  labelsOverride: Record<string, { headerName?: string }>,
) => {
  const filters = {} as Record<string, StringListFilterCategoryBuilder>;

  for (const [key, value] of Object.entries(response.baseDimensions)) {
    if (value.values.length === 0) continue;

    filters[`"${key}"`] = new StringListFilterCategoryBuilder()
      .setLabel(labelsOverride[key]?.headerName || key)
      .setOptions([
        { label: BLANK_VALUE, value: BLANK_VALUE },
        ...value.values.map((value) => {
          return { label: value, value: `"${value}"` };
        }),
      ]);
  }

  for (const [key, value] of Object.entries(response.labels)) {
    if (filters[key]) continue;
    if (filters[`"${key}"`]) continue;
    if (value.values.length === 0) continue;

    filters[`labels."${key}"`] = new StringListFilterCategoryBuilder()
      .setLabel(labelsOverride[key]?.headerName || key)
      .setOptions([
        { label: BLANK_VALUE, value: BLANK_VALUE },
        ...value.values.map((value) => {
          return { label: value, value: `"${value}"` };
        }),
      ]);
  }

  return filters;
};

/**
 * Converts the selected options to list of options.
 * Used as a placeholder in <MultiSelectFilter />, until the real data is received.
 * @param response SelectedOptions
 * @returns List of options based on the selectedOptions.
 */
export const filterOptionsPlaceholder = (
  selectedOptions: SelectedOptions,
  labelsOverride: Record<
    string,
    { header?: string | unknown; headerName?: string }
  >,
): OptionCategory[] => {
  return Object.entries(selectedOptions)
    .filter(([, values]) => Array.isArray(values))
    .map(([key, values]) =>
      filterOptionPlaceholder(key, values as string[], labelsOverride),
    );
};

const filterOptionPlaceholder = (
  key: string,
  values: string[],
  labelsOverride: Record<
    string,
    { header?: string | unknown; headerName?: string }
  >,
): OptionCategory => {
  const value = key;
  key = key.replace(/labels\."?(.*?)"?$/, '$1');
  const labelStr = getLabelFromOverride(key, labelsOverride);
  return {
    label: labelStr || key,
    value: value,
    options: values.map((value) => {
      return { label: value, value: value };
    }),
    type: 'string_list',
  };
};
