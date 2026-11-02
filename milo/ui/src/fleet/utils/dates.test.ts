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

import { DateTime } from 'luxon';

import { getValidDateTime, inferActivePresetId } from './dates';

describe('dates utilities', () => {
  it('infers active DateFilter quick preset across URL round-trips', () => {
    const now = DateTime.fromISO('2026-10-22T06:00:00.000Z');
    const fifteenMinAgo = now
      .minus({ minutes: 15 })
      .startOf('minute')
      .toJSDate();
    const thirtyMinAgo = now
      .minus({ minutes: 30 })
      .startOf('minute')
      .toJSDate();
    const twentyFourHoursAndFortyMinAgo = now
      .minus({ minutes: 24 * 60 + 40 })
      .startOf('minute')
      .toJSDate();

    expect(inferActivePresetId({ min: fifteenMinAgo }, now)).toBe('last_15m');
    expect(inferActivePresetId({ max: thirtyMinAgo }, now)).toBe('older_30m');
    expect(
      inferActivePresetId({ max: twentyFourHoursAndFortyMinAgo }, now),
    ).toBe('older_24h');
    expect(
      inferActivePresetId({ min: fifteenMinAgo, max: thirtyMinAgo }, now),
    ).toBeNull();
    expect(inferActivePresetId({}, now)).toBeNull();
  });

  it('validates Date objects with getValidDateTime', () => {
    expect(getValidDateTime(undefined)).toBeNull();
    expect(getValidDateTime(new Date('invalid-date'))).toBeNull();
    expect(
      getValidDateTime(new Date('2026-10-22T06:00:00.000Z'))?.toUTC().toISO(),
    ).toBe('2026-10-22T06:00:00.000Z');
    const dateOnlyDt = getValidDateTime(
      new Date('2026-10-22T00:00:00.000Z'),
      true,
    );
    expect(dateOnlyDt?.year).toBe(2026);
    expect(dateOnlyDt?.month).toBe(10);
    expect(dateOnlyDt?.day).toBe(22);
  });
});
