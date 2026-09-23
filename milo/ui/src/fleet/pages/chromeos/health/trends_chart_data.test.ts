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

import { TrendlineSeries } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  buildTimeAxisTicks,
  buildTrendsChartRows,
  buildValueAxisTicks,
  computeMaxYScale,
  findFreeColorSlot,
  MAX_VISIBLE_SERIES,
} from './trends_chart_data';

const at = (hour: number, minute = 0) =>
  `2026-09-14T${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}:00Z`;

const series = (
  name: string,
  points: readonly (readonly [string, number])[],
): TrendlineSeries => ({
  name,
  points: points.map(([timestamp, value]) => ({ timestamp, value })),
});

describe('buildTrendsChartRows', () => {
  it('returns no rows when nothing has been reported', () => {
    expect(buildTrendsChartRows([])).toEqual([]);
    expect(buildTrendsChartRows([series('empty', [])])).toEqual([]);
  });

  it('emits one row per hour and leaves unreported buckets empty', () => {
    const rows = buildTrendsChartRows([
      series('a', [
        [at(10), 0.9],
        [at(11), 0.95],
        [at(12), 0.97],
      ]),
      series('b', [
        [at(10), 0.8],
        [at(12), 0.82],
      ]),
    ]);

    expect(rows).toHaveLength(3);
    // `a` reported in all three buckets, so it draws an unbroken line.
    expect(rows.map((row) => row.values['a'])).toEqual([0.9, 0.95, 0.97]);
    // `b` genuinely has no reading in the middle bucket. It is absent rather
    // than interpolated, so the line breaks there.
    expect(rows.map((row) => row.values['b'])).toEqual([0.8, undefined, 0.82]);
  });

  it('does not treat another series\u0027 sample times as gaps in this one', () => {
    // The regression this port exists to fix. These two series are sampled
    // half an hour apart, so under the old union-of-timestamps model each of
    // them appeared to skip every other column and neither drew a line.
    const rows = buildTrendsChartRows([
      series('on-the-hour', [
        [at(10), 0.9],
        [at(11), 0.91],
        [at(12), 0.92],
      ]),
      series('on-the-half-hour', [
        [at(10, 30), 0.7],
        [at(11, 30), 0.71],
        [at(12, 30), 0.72],
      ]),
    ]);

    expect(rows).toHaveLength(3);
    for (const row of rows) {
      expect(typeof row.values['on-the-hour']).toBe('number');
      expect(typeof row.values['on-the-half-hour']).toBe('number');
    }
  });

  it('gives an outage width in proportion to how long it lasted', () => {
    const rows = buildTrendsChartRows([
      series('a', [
        [at(10), 0.9],
        [at(16), 0.4],
      ]),
    ]);

    // Six hours elapsed, so the gap spans six intervals rather than the single
    // step an index-based axis would have given it.
    expect(rows).toHaveLength(7);
    expect(rows[0].values['a']).toBe(0.9);
    expect(rows[6].values['a']).toBe(0.4);
    for (const row of rows.slice(1, 6)) {
      expect(row.values).toEqual({});
    }

    const elapsed = rows[6].timestampMs - rows[0].timestampMs;
    expect(elapsed).toBe(6 * 60 * 60 * 1000);
  });

  it('assigns every point in the same hour to the same bucket', () => {
    const rows = buildTrendsChartRows([
      series('a', [
        [at(10, 1), 0.9],
        [at(10, 59), 0.95],
      ]),
    ]);

    expect(rows).toHaveLength(1);
    expect(rows[0].values['a']).toBe(0.95);
  });

  it('clamps a negative reading to zero', () => {
    // The value axis starts at zero, so a negative would stretch it below the
    // range the ticks label.
    const rows = buildTrendsChartRows([series('a', [[at(10), -0.2]])]);
    expect(rows[0].values['a']).toBe(0);
  });

  it('ignores points with a missing or unparsable timestamp', () => {
    const rows = buildTrendsChartRows([
      {
        name: 'a',
        points: [
          { timestamp: undefined, value: 0.5 },
          { timestamp: 'not a timestamp', value: 0.6 },
          { timestamp: at(10), value: 0.9 },
        ],
      },
    ]);

    expect(rows).toHaveLength(1);
    expect(rows[0].values['a']).toBe(0.9);
  });

  it('stays finite when a stray timestamp implies an implausible span', () => {
    const rows = buildTrendsChartRows([
      series('a', [
        ['1970-01-01T00:00:00Z', 0.1],
        [at(10), 0.9],
      ]),
    ]);

    // Filling every hour since the epoch would hang the browser, so only the
    // buckets carrying data are plotted.
    expect(rows).toHaveLength(2);
    expect(rows[0].timestampMs).toBeLessThan(rows[1].timestampMs);
  });
});

describe('findFreeColorSlot', () => {
  it('hands out the lowest free slot', () => {
    expect(findFreeColorSlot({})).toBe(0);
    expect(findFreeColorSlot({ a: 0, b: 1 })).toBe(2);
  });

  it('reuses a slot once its series is deselected', () => {
    expect(findFreeColorSlot({ a: 0, b: 1, c: 2 })).toBe(3);
    expect(findFreeColorSlot({ a: 0, c: 2 })).toBe(1);
  });

  it('reports exhaustion once every palette slot is taken', () => {
    const full = Object.fromEntries(
      Array.from({ length: MAX_VISIBLE_SERIES }, (_, i) => [`s${i}`, i]),
    );
    expect(findFreeColorSlot(full)).toBeUndefined();
  });
});

describe('axis helpers', () => {
  it('labels first and last bucket, and thins the rest', () => {
    const rows = buildTrendsChartRows([
      series(
        'a',
        Array.from({ length: 24 }, (_, i) => [at(i), 0.9] as const),
      ),
    ]);

    const ticks = buildTimeAxisTicks(rows);

    expect(ticks[0]).toBe(rows[0].timestampMs);
    expect(ticks[ticks.length - 1]).toBe(rows[rows.length - 1].timestampMs);
    expect(ticks.length).toBeLessThanOrEqual(7);
    expect([...ticks]).toEqual([...ticks].sort((a, b) => a - b));
  });

  it('labels every bucket when there are only a few', () => {
    const rows = buildTrendsChartRows([
      series('a', [
        [at(10), 0.9],
        [at(11), 0.9],
      ]),
    ]);
    expect(buildTimeAxisTicks(rows)).toHaveLength(2);
  });

  it('keeps the value axis at 100% until a series exceeds it', () => {
    expect(computeMaxYScale([series('a', [[at(10), 0.9]])])).toBe(1);
    expect(computeMaxYScale([series('a', [[at(10), 1.05]])])).toBe(1.2);
  });

  it('spreads six value ticks across the axis', () => {
    expect(buildValueAxisTicks(1)).toEqual([0, 0.2, 0.4, 0.6, 0.8, 1]);
    expect(buildValueAxisTicks(1.2)).toEqual([0, 0.24, 0.48, 0.72, 0.96, 1.2]);
  });
});
