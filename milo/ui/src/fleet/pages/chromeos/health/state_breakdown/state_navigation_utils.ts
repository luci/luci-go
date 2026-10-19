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

import { healthToDeviceFilter } from '@/fleet/adapters/filter_adapters';
import { FILTERS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { combineAipFiltersWithParentheses } from '@/fleet/utils/search_param';

import { ExactDutStateKey } from './health_state_definitions';

/**
 * Cross-domain adapter converting Health Dashboard AIP-160 filter
 * clauses to Devices List compatible syntax using AST remapping.
 */
export const adaptHealthFilterToDeviceList = healthToDeviceFilter;

/**
 * Builds the URL to the ChromeOS devices list pre-filtered by the exact DUT state
 * and adapted cohort filter.
 */
export const getDeviceListStateUrl = (
  stateKey: ExactDutStateKey,
  baseFilter?: string,
): string => {
  const stateClause = `labels."dut_state" = "${stateKey}"`;
  const adaptedBaseFilter = healthToDeviceFilter(baseFilter);
  const combinedFilter = combineAipFiltersWithParentheses(
    adaptedBaseFilter,
    stateClause,
  );
  return `/ui/fleet/p/chromeos/devices?${FILTERS_PARAM_KEY}=${encodeURIComponent(
    combinedFilter,
  )}`;
};
