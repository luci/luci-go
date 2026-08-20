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

import _ from 'lodash';
import { useCallback, useMemo } from 'react';

import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import {
  useFilters,
  FilterCategory,
} from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { ChromeOSFilterKey } from './chromeos_fields';
import { useChromeOSFields } from './use_chromeos_available_columns';

export const useChromeOSFilterBuilders = (): {
  filterBuilders: Record<ChromeOSFilterKey, StringListFilterCategoryBuilder>;
  isLoading: boolean;
} => {
  const { availableFields, getValues, isLoading } = useChromeOSFields();

  const filterBuilders = useMemo(() => {
    const filters = {} as Record<
      ChromeOSFilterKey,
      StringListFilterCategoryBuilder
    >;

    availableFields.forEach((def) => {
      const values = getValues(def.id);
      if (values.length === 0) return;

      filters[def.filterKey] = new StringListFilterCategoryBuilder()
        .setLabel(def.header)
        .setOptions([
          { label: BLANK_VALUE, value: BLANK_VALUE },
          ...values.map((v) => ({ label: v, value: v })),
        ]);
    });

    return filters;
  }, [availableFields, getValues]);

  return {
    filterBuilders,
    isLoading,
  };
};

export const useChromeOSFilters = (
  onApply?: (searchParams: URLSearchParams) => URLSearchParams | void,
): {
  filterValues: Record<ChromeOSFilterKey, FilterCategory> | undefined;
  aip160: () => string;
  isLoading: boolean;
  warnings: string[];
  setFiltersBatch: (updates: Record<string, string[]>) => void;
} => {
  const { filterBuilders, isLoading } = useChromeOSFilterBuilders();
  const { trackEvent } = useGoogleAnalytics();

  const onApplyFilter = useCallback(
    (searchParams: URLSearchParams) => {
      trackEvent('filter_changed', {
        componentName: 'device_list_filter',
      });
      return onApply?.(searchParams) ?? searchParams;
    },
    [onApply, trackEvent],
  );

  const { filterValues, aip160, warnings, setFiltersBatch } = useFilters(
    filterBuilders,
    {
      areFilterValuesLoading: isLoading,
      onFilterChange: onApplyFilter,
    },
  );

  return {
    filterValues: filterValues as
      | Record<ChromeOSFilterKey, FilterCategory>
      | undefined,
    aip160,
    isLoading,
    warnings,
    setFiltersBatch,
  };
};
