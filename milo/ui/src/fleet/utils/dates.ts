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

import { DateOnly } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/common_types.pb';

export const toIsoString = (dateOnly: DateOnly | undefined): string => {
  if (!dateOnly) {
    return '';
  }
  return `${dateOnly.year}-${String(dateOnly.month).padStart(2, '0')}-${String(dateOnly.day).padStart(2, '0')}`;
};

export const toLuxonDateTime = (
  dateOnly: DateOnly | undefined,
): DateTime | undefined => {
  if (!dateOnly) {
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
  if (!dateTime) {
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
