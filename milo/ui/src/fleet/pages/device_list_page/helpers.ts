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

import { OptionCategory } from '@/fleet/types';
import { GetDeviceDimensionsResponse } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

/**
 * Converts a response from GetDeviceDimensions into a list of options
 * for <MultiSelectFilter />
 * @param response GetDeviceDimensionsResponse
 * @returns List of options based on response data.
 */
export const dimensionsToFilterOptions = (
  response: GetDeviceDimensionsResponse,
): OptionCategory[] => {
  const baseDimensions = Object.entries(response.baseDimensions).map(
    ([key, value]) => {
      return {
        label: key,
        value: key,
        options: value.values.map((value) => {
          return { label: value, value: value };
        }),
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
        label: key,
        value: 'labels.' + key,
        options: value.values.map((value) => {
          return { label: value, value: value };
        }),
      } as OptionCategory,
    ];
  });

  return baseDimensions.concat(labels).filter((o) => o.options.length > 0);
};
