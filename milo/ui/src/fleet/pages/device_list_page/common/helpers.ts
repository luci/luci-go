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

import { BLANK_VALUE } from '@/fleet/constants/filters';
import { OptionCategory, SelectedOptions } from '@/fleet/types';
import { GetDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

/**
 * Converts a response from GetDeviceDimensions into a list of options
 * for <MultiSelectFilter />
 * @param response GetDeviceDimensionsResponse
 * @returns List of options based on response data.
 */
export const dimensionsToFilterOptions = (
  response: GetDeviceDimensionsResponse,
  labelsOverride: Record<string, { headerName?: string }>, // TODO: should this be columns?
): OptionCategory[] => {
  const baseDimensions = Object.entries(response.baseDimensions).map(
    ([key, value]) => {
      return {
        label: labelsOverride[key]?.headerName || key,
        value: key,
        options: [
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...value.values.map((value) => {
            return { label: value, value: value };
          }),
        ],
      } as OptionCategory;
    },
  );

  const labels = Object.entries(response.labels).flatMap(([key, value]) => {
    // We need to avoid duplicate options
    // E.g. `dut_id` is in both base dimensions and labels
    if (response.baseDimensions[key]) {
      return [];
    }

    return [
      {
        label: labelsOverride[key]?.headerName || key,
        value: `labels."${key}"`,
        options: [
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...value.values.map((value) => {
            return { label: value, value: value };
          }),
        ],
      } as OptionCategory,
    ];
  });

  return baseDimensions
    .concat(labels)
    .sort((a, b) => a.label.localeCompare(b.label)) // Sort alphabetically
    .filter((o) => o.options.length > 0);
};

/**
 * Converts the selected options to list of options.
 * Used as a placeholder in <MultiSelectFilter />, until the real data is received.
 * @param response SelectedOptions
 * @returns List of options based on the selectedOptions.
 */
export const filterOptionsPlaceholder = (
  selectedOptions: SelectedOptions,
  labelsOverride: Record<string, { headerName?: string }>,
): OptionCategory[] => {
  return Object.entries(selectedOptions)
    .map(([key, values]) =>
      filterOptionPlaceholder(key, values, labelsOverride),
    )
    .sort((a, b) => a.label.localeCompare(b.label)); // Sort alphabetically
};

const filterOptionPlaceholder = (
  key: string,
  values: string[],
  labelsOverride: Record<string, { headerName?: string }>,
): OptionCategory => {
  const value = key;
  key = key.replace(/labels\."?(.*?)"?$/, '$1');
  return {
    label: labelsOverride?.[key]?.headerName || key,
    value: value,
    options: values.map((value) => {
      return { label: value, value: value };
    }),
  };
};
