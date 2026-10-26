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

import { RangeFilterCategoryBuilder } from '@/fleet/components/filters/range_filter';
import { useChromeOSFields } from '@/fleet/pages/device_list_page/chromeos/use_chromeos_available_columns';

import {
  buildFilterBuilders,
  buildStringListFilter,
  RepairQueueFilterBuilder,
} from './repair_filter_builders';
import { useIrmIncidents } from './use_irm_incidents';

export type { RepairQueueFilterBuilder };
export type RepairQueueFilterKey = string;

export const POOL_HEALTH_FILTER_KEY = 'pool_health_pct';
export const MODEL_HEALTH_FILTER_KEY = 'model_health_pct';
export const ACTIVE_IRM_FILTER_KEY = 'active_irm_ids';

/**
 * Filter builders for ChromeOS repair queue priority scoring rules
 * (`RepairTasksFilterMap`), extending device attribute filters with pool/model
 * health percentages and `active_irm_ids`.
 */
export const useRepairQueueFilterBuilders = (): {
  filterBuilders: Record<string, RepairQueueFilterBuilder>;
  isLoading: boolean;
} => {
  const {
    availableFields,
    getValues,
    isLoading: isFieldsLoading,
  } = useChromeOSFields();
  const { data: irmData, isLoading: isIrmLoading } = useIrmIncidents();

  const filterBuilders = useMemo(() => {
    const irmBugIds = Array.from(
      new Set(
        (irmData?.irmIncidents ?? [])
          .map((inc) => inc.masterBugId?.trim())
          .filter((id): id is string => Boolean(id)),
      ),
    );

    const queueFilters: Record<string, RepairQueueFilterBuilder> = {
      [POOL_HEALTH_FILTER_KEY]: new RangeFilterCategoryBuilder()
        .setLabel('Pool Health')
        .setMin(0)
        .setMax(100)
        .setScaleFactor(0.01)
        .setUnit('%'),
      [MODEL_HEALTH_FILTER_KEY]: new RangeFilterCategoryBuilder()
        .setLabel('Model Health')
        .setMin(0)
        .setMax(100)
        .setScaleFactor(0.01)
        .setUnit('%'),
      [ACTIVE_IRM_FILTER_KEY]: buildStringListFilter(
        'Active IRM',
        irmBugIds,
        (bugId) => `b/${bugId}`,
      ),
    };

    return buildFilterBuilders(availableFields, getValues, queueFilters);
  }, [availableFields, getValues, irmData]);

  return {
    filterBuilders,
    isLoading: isFieldsLoading || isIrmLoading,
  };
};
