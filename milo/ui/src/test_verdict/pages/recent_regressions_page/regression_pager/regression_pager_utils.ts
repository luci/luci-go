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

import { DateTime } from 'luxon';

import { searchParamUpdater } from '@/generic_libs/hooks/synced_search_params';

/**
 * getWeek extract week from search parameters.
 */
export function getWeek(searchParams: URLSearchParams, now: DateTime) {
  const week = searchParams.get('wk');
  return week
    ? truncateToBeginOfWeek(DateTime.fromSeconds(parseInt(week)).toUTC())
    : truncateToBeginOfWeek(now);
}

function truncateToBeginOfWeek(time: DateTime) {
  // Week is defined as from Sunday to Saturday.
  return time.toUTC().plus({ day: 1 }).startOf('week').minus({ day: 1 });
}

/**
 * Do not export outside of regression_pager.
 * weekUpdater updates week in search parameters.
 */
export function weekUpdater(newWeek: DateTime) {
  return searchParamUpdater('wk', newWeek.toUnixInteger().toString());
}
