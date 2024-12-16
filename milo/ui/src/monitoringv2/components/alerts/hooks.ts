// Copyright 2024 The LUCI Authors.
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
  searchParamUpdater,
  useSyncedSearchParams,
} from '@/generic_libs/hooks/synced_search_params';

const ALERT_FILTER_PARAM = 'q';
const SELECTED_TAB_PARAM = 'alerts_tab';
export const DEFAULT_ALERT_TAB = 'ungrouped';

export function useFilterQuery(defaultValue?: string) {
  const [searchParams, setURLSearchParams] = useSyncedSearchParams();
  return [
    searchParams.get(ALERT_FILTER_PARAM) || defaultValue,
    (value: string) => {
      if (value === defaultValue) {
        searchParams.delete(ALERT_FILTER_PARAM);
        setURLSearchParams(searchParams);
      } else {
        setURLSearchParams(searchParamUpdater(ALERT_FILTER_PARAM, value));
      }
    },
  ] as [string, (val: string) => void];
}

export function useSelectedTab(defaultValue?: string) {
  const [searchParams, setURLSearchParams] = useSyncedSearchParams();
  return [
    searchParams.get(SELECTED_TAB_PARAM) || defaultValue,
    (value: string) => {
      setURLSearchParams(searchParamUpdater(SELECTED_TAB_PARAM, value));
    },
  ] as [string, (val: string) => void];
}
