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

import { generateDeviceListURL } from '@/fleet/constants/paths';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';
import { AndroidRepairQueueItem } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { CLAIMED_BY_FILTER_KEY } from './use_android_priority_rule_filter_builders';

const ID_FILTER_KEY = '"id"';

/**
 * Returns the AIP-160 repair queue filter matching the tasks claimed by
 * `email`.
 */
export const getClaimedByFilter = (email: string): string =>
  `${CLAIMED_BY_FILTER_KEY} = ${JSON.stringify(email)}`;

/**
 * Returns the Fleet Console device list of `workspace` filtered down to the
 * `deviceId`s of `items`. Returns an empty string when `items` is empty.
 */
export const getDeviceListLinkForRepairTasks = (
  workspace: AndroidPageWorkspace,
  items: readonly AndroidRepairQueueItem[],
): string => {
  const ids = [...new Set(items.map((item) => item.deviceId).filter(Boolean))];
  if (!ids.length) return '';

  const filters: Record<string, string[]> = { [ID_FILTER_KEY]: ids };

  return `${generateDeviceListURL(workspaces[workspace].baseUrl)}${getFilterQueryString(filters, undefined)}`;
};
