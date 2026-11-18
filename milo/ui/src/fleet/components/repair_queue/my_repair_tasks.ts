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

import { UNKNOWN_USER_EMAIL } from './use_current_user_email';

const getTrimmedDeviceId = (item: { readonly deviceId: string }): string =>
  item.deviceId.trim();

/**
 * Filters `items` to repair tasks claimed by `currentUser` that carry a
 * non-empty `deviceId`, keeping task counts aligned with linkable device rows
 * while preserving distinct host and device tasks that share a `deviceId` on
 * Android/Pixel.
 */
export const getMyClaimedRepairTasks = <
  T extends { readonly deviceId: string; readonly claimedBy?: string },
>(
  items: readonly T[],
  currentUser: string,
): T[] => {
  if (!currentUser || currentUser === UNKNOWN_USER_EMAIL) {
    return [];
  }
  return items.filter(
    (item) =>
      item.claimedBy === currentUser && Boolean(getTrimmedDeviceId(item)),
  );
};

/**
 * Returns the Fleet Console device list URL for `platformBaseUrl` filtered down
 * to the deduplicated non-empty `deviceId`s of `items`. Returns `''` when no
 * valid `deviceId` is present.
 */
export const buildRepairTasksDeviceListLink = (
  items: readonly { readonly deviceId: string }[],
  platformBaseUrl: string,
  idFilterKey = 'id',
): string => {
  const ids = [...new Set(items.map(getTrimmedDeviceId).filter(Boolean))];
  if (!ids.length) return '';

  const filters: Record<string, string[]> = { [idFilterKey]: ids };
  return `${generateDeviceListURL(platformBaseUrl)}${getFilterQueryString(filters, undefined)}`;
};
