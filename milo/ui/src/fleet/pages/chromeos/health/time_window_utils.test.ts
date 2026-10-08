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

import {
  getHourAlignedTimeWindow,
  RANKING_HOURS_WINDOW,
  RECENT_HOURS_WINDOW,
} from './time_window_utils';

describe('time_window_utils', () => {
  it('defines expected window constants', () => {
    expect(RECENT_HOURS_WINDOW).toBe(2);
    expect(RANKING_HOURS_WINDOW).toBe(26);
  });

  it('rounds endTime down to the start of the current hour', () => {
    // 2026-10-07T11:25:39.123Z
    const testNow = new Date('2026-10-07T11:25:39.123Z').getTime();
    const window = getHourAlignedTimeWindow(RECENT_HOURS_WINDOW, testNow);

    expect(window.endTime).toBe('2026-10-07T11:00:00.000Z');
    expect(window.startTime).toBe('2026-10-07T09:00:00.000Z');
  });

  it('calculates startTime exactly hoursBack before endTime', () => {
    const testNow = new Date('2026-10-07T11:00:00.000Z').getTime();
    const window = getHourAlignedTimeWindow(RANKING_HOURS_WINDOW, testNow);

    expect(window.endTime).toBe('2026-10-07T11:00:00.000Z');
    // 26 hours back from Oct 7, 11:00 is Oct 6, 09:00
    expect(window.startTime).toBe('2026-10-06T09:00:00.000Z');
  });

  it('guarantees identical query keys for sub-second and minute drifts within the same hour', () => {
    const time1 = new Date('2026-10-07T11:05:12.050Z').getTime();
    const time2 = new Date('2026-10-07T11:45:59.999Z').getTime();

    const window1 = getHourAlignedTimeWindow(RECENT_HOURS_WINDOW, time1);
    const window2 = getHourAlignedTimeWindow(RECENT_HOURS_WINDOW, time2);

    expect(window1.startTime).toBe(window2.startTime);
    expect(window1.endTime).toBe(window2.endTime);
  });

  it('defaults to current wall clock time when nowMs is omitted', () => {
    const window = getHourAlignedTimeWindow(2);
    const endDate = new Date(window.endTime);

    expect(endDate.getMinutes()).toBe(0);
    expect(endDate.getSeconds()).toBe(0);
    expect(endDate.getMilliseconds()).toBe(0);
  });
});
