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

import { Timestamp } from '@/crystal_ball/types';

/**
 * Helper to convert DateTime to Timestamp.
 * @param date - raw value from the DateTimePicker.
 * @returns date in Timestamp proto format.
 */
export function dateToTimestamp(date: DateTime | null): Timestamp | undefined {
  if (!date) return undefined;
  const seconds = date.toSeconds();
  const nanos = 0;
  return { seconds, nanos };
}

/**
 * Helper to convert Timestamp to DateTime.
 * @param timestamp - raw Timestamp proto value.
 * @returns timestamp value converted to a DateTime value.
 */
export function timestampToDate(
  timestamp: Timestamp | undefined,
): DateTime | null {
  if (!timestamp || timestamp.seconds === undefined) return null;
  return DateTime.fromSeconds(timestamp.seconds).toUTC();
}
