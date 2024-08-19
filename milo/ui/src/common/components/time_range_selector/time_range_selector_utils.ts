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

import { DateTime, Duration } from 'luxon';

import { searchParamUpdater } from '@/generic_libs/hooks/synced_search_params';

/**
 * The relative time range options that are currently supported in the menu and the URL parameter.
 * TODO (beining@): It will be better if we can support any number of days within a sensible range in the URL parameter.
 * This is to provide a consistent interface of this API.
 */
export const RELATIVE_TIME_OPTIONS = {
  '1d': {
    duration: Duration.fromObject({ days: 1 }),
    displayText: 'Last 1 day',
  },
  '3d': {
    duration: Duration.fromObject({ days: 3 }),
    displayText: 'Last 3 days',
  },
  '7d': {
    duration: Duration.fromObject({ days: 7 }),
    displayText: 'Last 7 days',
  },
} as const;

export const CUSTOMIZE_OPTION = 'customize';
export const CUSTOMIZE_OPTION_DISPLAY_TEXT = 'Customize time range';

export type MenuOption =
  | typeof CUSTOMIZE_OPTION
  | keyof typeof RELATIVE_TIME_OPTIONS;

export const DEFAULT_MENU_OPTION: MenuOption = '3d';

export const OPTION_KEY = 'time_option';
export const START_TIME_KEY = 'start_time';
export const END_TIME_KEY = 'end_time';

export const getSelectedOption = (params: URLSearchParams): MenuOption =>
  (params.get(OPTION_KEY) as MenuOption) || DEFAULT_MENU_OPTION;

export function optionParamUpdater(value: MenuOption) {
  return searchParamUpdater(OPTION_KEY, value, DEFAULT_MENU_OPTION.toString());
}

export const getStartEndTime = (params: URLSearchParams) => {
  const start = params.get(START_TIME_KEY);
  const end = params.get(END_TIME_KEY);
  return {
    startTime: start ? DateTime.fromSeconds(parseInt(start)).toUTC() : null,
    endTime: end ? DateTime.fromSeconds(parseInt(end)).toUTC() : null,
  };
};

export function timeRangeParamUpdater(
  key: typeof END_TIME_KEY | typeof START_TIME_KEY,
  value: DateTime | null,
) {
  return searchParamUpdater(key, value?.toUnixInteger().toString() || null);
}

/**
 * Client can use getAbsoluteStartEndTime to obtain the start and end based
 * the current time range selected.
 */
export const getAbsoluteStartEndTime = (
  params: URLSearchParams,
  now: DateTime,
) => {
  const option = getSelectedOption(params);
  if (option === CUSTOMIZE_OPTION) {
    return getStartEndTime(params);
  }
  const endTime = now;
  const startTime = now.minus(RELATIVE_TIME_OPTIONS[option].duration);
  return { startTime, endTime };
};
