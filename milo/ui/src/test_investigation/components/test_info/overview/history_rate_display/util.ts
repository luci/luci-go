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

import { DateTime } from 'luxon';

import { displayApproxDuartion } from '@/common/tools/time_utils';

/**
 * Given an ISO string, returns a human-readable string representing how long
 * ago that time was.  Handles details specific to history segment display.
 *
 * @param isoString The ISO string to format.
 * @param nowDateTime The current time. Defaults to `DateTime.now()`.
 */
export function formatSegmentTimestamp(
  isoString?: string,
  nowDateTime: DateTime = DateTime.now(),
): string | undefined {
  if (!isoString) return undefined;
  const pastDateTime = DateTime.fromISO(isoString, { zone: 'utc' });
  if (!pastDateTime.isValid) return undefined;
  const approxDurationText = displayApproxDuartion(
    nowDateTime.diff(pastDateTime),
  );
  return approxDurationText && approxDurationText !== 'N/A'
    ? `${approxDurationText} ago`
    : undefined;
}
