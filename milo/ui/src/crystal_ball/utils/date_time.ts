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

import { Timestamp } from '@/proto/google/protobuf/timestamp.pb';

/**
 * Helper to convert Timestamp to DateTime.
 * @param timestamp - raw Timestamp proto value.
 * @returns timestamp value converted to a DateTime value.
 */
export function timestampToDate(
  timestamp: Timestamp | undefined,
): DateTime | null {
  if (!timestamp || timestamp.seconds === undefined) return null;
  return DateTime.fromSeconds(parseFloat(timestamp.seconds)).toUTC();
}

/**
 * Helper to format a string or Timestamp into a relative time string.
 * @param val - an ISO string or Timestamp object.
 * @returns relative time string (e.g. "3 days ago").
 */
export function formatRelativeTime(val?: string | Timestamp): string {
  if (!val) return 'Unknown';
  try {
    let dt: DateTime | null = null;
    if (typeof val === 'string') {
      dt = DateTime.fromISO(val);
    } else if (val.seconds !== undefined) {
      dt = DateTime.fromSeconds(parseFloat(val.seconds)).toUTC();
    }
    if (!dt || !dt.isValid) throw new Error('Invalid date');
    return dt.toRelative() || '';
  } catch {
    return 'Invalid date';
  }
}

/**
 * Formats a date-time value (string, number epoch, or Date object) in a specified timezone.
 * Returns standard YYYY-MM-DD HH:mm:ss format.
 */
export function formatTimestampWithZone(
  ts: string | number | Date | undefined | null,
  timeZone: string,
): string {
  if (ts === undefined || ts === null || ts === '') return '';
  const date = ts instanceof Date ? ts : new Date(ts);
  if (isNaN(date.getTime())) return String(ts);

  try {
    const formatter = new Intl.DateTimeFormat('sv-SE', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
      timeZone,
    });
    return formatter.format(date);
  } catch {
    return date.toLocaleString();
  }
}

/**
 * Returns date and time formatting instances configured for a specific timezone.
 * Useful for bulk formatting operations where construction overhead must be minimized.
 */
export function getTimeFormatters(timeZone: string) {
  try {
    const dateFmt = new Intl.DateTimeFormat('sv-SE', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      timeZone,
    });
    const timeFmt = new Intl.DateTimeFormat(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      timeZone,
    });
    return { dateFmt, timeFmt };
  } catch {
    return null;
  }
}
