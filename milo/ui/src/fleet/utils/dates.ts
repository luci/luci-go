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

import { DateTime, Duration } from 'luxon';

import { DateFilterValue } from '@/fleet/types';
import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

export interface QuickRelativePreset {
  id: string;
  label: string;
  bound: 'min' | 'max';
  minutesAgo: number;
}

export const QUICK_RELATIVE_PRESETS: QuickRelativePreset[] = [
  { id: 'last_15m', label: 'Last 15m', bound: 'min', minutesAgo: 15 },
  { id: 'last_1h', label: 'Last 1h', bound: 'min', minutesAgo: 60 },
  { id: 'older_30m', label: '> 30m ago', bound: 'max', minutesAgo: 30 },
  { id: 'older_1h', label: '> 1h ago', bound: 'max', minutesAgo: 60 },
  { id: 'older_24h', label: '> 24h ago', bound: 'max', minutesAgo: 24 * 60 },
];

export const getValidDateTime = (
  date?: Date,
  isDateOnly = false,
): DateTime<true> | null => {
  if (!date || isNaN(date.getTime())) return null;
  const dateTime = isDateOnly
    ? DateTime.fromJSDate(date, { zone: 'utc' }).setZone('default', {
        keepLocalTime: true,
      })
    : DateTime.fromJSDate(date);
  return dateTime.isValid ? dateTime : null;
};

/**
 * Infers which quick relative preset matches the current DateFilterValue,
 * allowing preset chip selection state to survive URL round-trips when
 * reopening an applied filter chip or dropdown.
 */
export function inferActivePresetId(
  value: DateFilterValue,
  now: DateTime = DateTime.now(),
): string | null {
  const minDate = getValidDateTime(value.min);
  const maxDate = getValidDateTime(value.max);
  if (Boolean(minDate) === Boolean(maxDate)) {
    return null;
  }
  const bound: 'min' | 'max' = minDate ? 'min' : 'max';
  const targetDate = (minDate ?? maxDate)!;
  const diffMinutes = now
    .startOf('minute')
    .diff(targetDate.startOf('minute'), 'minutes').minutes;

  let bestPresetId: string | null = null;
  let bestDelta = Infinity;
  for (const preset of QUICK_RELATIVE_PRESETS) {
    if (preset.bound !== bound) continue;
    const delta = Math.abs(diffMinutes - preset.minutesAgo);
    const maxForwardTolerance = Math.max(
      5,
      Math.round(preset.minutesAgo * 0.1),
    );
    if (
      diffMinutes >= preset.minutesAgo - 1 &&
      diffMinutes <= preset.minutesAgo + maxForwardTolerance &&
      delta < bestDelta
    ) {
      bestDelta = delta;
      bestPresetId = preset.id;
    }
  }
  return bestPresetId;
}

export const toIsoString = (dateOnly: DateOnly | undefined): string => {
  if (!dateOnly) {
    return '';
  }
  return `${dateOnly.year}-${String(dateOnly.month).padStart(2, '0')}-${String(dateOnly.day).padStart(2, '0')}`;
};

export const toLuxonDateTime = (
  dateOnly: DateOnly | undefined,
): DateTime | undefined => {
  if (
    !dateOnly ||
    isNaN(dateOnly.year!) ||
    isNaN(dateOnly.month!) ||
    isNaN(dateOnly.day!)
  ) {
    return undefined;
  }

  return DateTime.fromObject({
    year: dateOnly.year,
    month: dateOnly.month,
    day: dateOnly.day,
  });
};

export const fromLuxonDateTime = (
  dateTime: DateTime | null | undefined,
): DateOnly | undefined => {
  if (!dateTime || !dateTime.isValid) {
    return undefined;
  }

  return {
    year: dateTime.year,
    month: dateTime.month,
    day: dateTime.day,
  };
};

export const prettyDateTime = (dt: string | undefined): string => {
  if (!dt) {
    return '';
  }
  const asObj = DateTime.fromISO(dt);
  return `${asObj.toLocaleString(DateTime.DATETIME_SHORT_WITH_SECONDS)} (${asObj.zoneName})`;
};

export const prettySeconds = (seconds: number | undefined): string => {
  if (!seconds) {
    return '0s';
  }
  return Duration.fromObject({ seconds: seconds })
    .rescale()
    .toHuman({ unitDisplay: 'short' });
};
