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

import { calculate24hDelta } from './ranking_utils';

describe('calculate24hDelta', () => {
  it('returns default zero values for empty or undefined points', () => {
    expect(calculate24hDelta([])).toEqual({
      availabilityPct: 0,
      prevAvailabilityPct: 0,
      healthDrop: 0,
      healthGain: 0,
      trend: 'flat',
    });
  });

  it('calculates upward trend with healthGain > 0 and healthDrop = 0', () => {
    const points = [
      { timestamp: '2026-09-29T10:00:00Z', value: 0.82 },
      { timestamp: '2026-09-30T10:00:00Z', value: 0.91 },
    ];

    const result = calculate24hDelta(points);
    expect(result).toEqual({
      availabilityPct: 91,
      prevAvailabilityPct: 82,
      healthDrop: 0,
      healthGain: 9,
      trend: 'up',
    });
  });

  it('correctly handles surplus availability exceeding 100% without capping at 100%', () => {
    const points = [
      { timestamp: '2026-09-29T12:00:00Z', value: 1.3 }, // 130%
      { timestamp: '2026-09-30T12:00:00Z', value: 1.25 }, // 125%
    ];

    const result = calculate24hDelta(points);
    expect(result.availabilityPct).toBe(125);
    expect(result.prevAvailabilityPct).toBe(130);
    expect(result.healthDrop).toBe(5);
    expect(result.healthGain).toBe(0);
    expect(result.trend).toBe('down');
  });

  it('handles surplus availability improving above 100%', () => {
    const points = [
      { timestamp: '2026-09-29T12:00:00Z', value: 1.05 }, // 105%
      { timestamp: '2026-09-30T12:00:00Z', value: 1.2 }, // 120%
    ];

    const result = calculate24hDelta(points);
    expect(result.availabilityPct).toBe(120);
    expect(result.prevAvailabilityPct).toBe(105);
    expect(result.healthDrop).toBe(0);
    expect(result.healthGain).toBe(15);
    expect(result.trend).toBe('up');
  });

  it('compares with point closest to 24h prior', () => {
    const points = [
      { timestamp: '2026-09-29T11:55:00Z', value: 0.8 },
      { timestamp: '2026-09-30T00:00:00Z', value: 0.5 },
      { timestamp: '2026-09-30T12:00:00Z', value: 0.75 },
    ];

    const result = calculate24hDelta(points);
    expect(result.availabilityPct).toBe(75);
    expect(result.prevAvailabilityPct).toBe(80);
    expect(result.healthDrop).toBe(5);
    expect(result.healthGain).toBe(0);
    expect(result.trend).toBe('down');
  });

  it('falls back to first point if points lack timestamps', () => {
    const points = [{ value: 0.7 }, { value: 0.7 }];

    const result = calculate24hDelta(points);
    expect(result.availabilityPct).toBe(70);
    expect(result.prevAvailabilityPct).toBe(70);
    expect(result.healthDrop).toBe(0);
    expect(result.healthGain).toBe(0);
    expect(result.trend).toBe('flat');
  });

  it('clamps negative values to 0 without capping upper bound', () => {
    const points = [{ timestamp: '2026-09-30T12:00:00Z', value: -0.1 }];

    const result = calculate24hDelta(points);
    expect(result.availabilityPct).toBe(0);
    expect(result.prevAvailabilityPct).toBe(0);
  });
});
