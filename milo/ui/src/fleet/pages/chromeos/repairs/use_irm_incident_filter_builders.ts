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

import { useMemo } from 'react';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';

import { buildFilterBuilders } from './repair_filter_builders';

/**
 * Filter builders for IRM incident definitions (`IrmIncidentFilterMap`).
 * Offers only device hardware attributes and labels, excluding repair queue
 * health metrics (`pool_health_pct`, `model_health_pct`) and `active_irm_ids`.
 */
export const useIrmIncidentFilterBuilders = (): {
  filterBuilders: Record<string, StringListFilterCategoryBuilder>;
  isLoading: boolean;
} => {
  const { availableFields, getValues, isLoading } = useChromeOSFields();

  const filterBuilders = useMemo(
    () => buildFilterBuilders(availableFields, getValues),
    [availableFields, getValues],
  );

  return {
    filterBuilders,
    isLoading,
  };
};
