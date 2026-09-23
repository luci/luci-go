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

import { colors } from '@/fleet/theme/colors';
import { TrendlineSeries } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export const PALETTE = [
  colors.blue[600],
  colors.green[600],
  colors.yellow[600],
  colors.red[600],
  colors.purple[600],
  colors.cyan[600],
  colors.orange[600],
  colors.pink[600],
];

/**
 * At most one series per palette entry is plotted, so the size of the palette
 * is also the maximum number of series that can be shown at once.
 */
export const MAX_VISIBLE_SERIES = PALETTE.length;

/** How many series are plotted before the user picks their own. */
export const DEFAULT_VISIBLE_SERIES = 5;

/**
 * Maps a series name to the palette slot it owns. A name is present if and
 * only if that series is plotted, and the value is an index into `PALETTE`.
 *
 * Slots are handed out when the user selects a series and are keyed by name
 * rather than by position in the RPC response. A background refetch that
 * reorders, adds or drops series therefore cannot recolor the series already
 * on the chart, and a series that temporarily disappears from the response
 * keeps its color when it comes back.
 */
export type SeriesColorSlots = Record<string, number>;

/**
 * Returns the lowest palette slot not currently owned by a series, or
 * undefined when every slot is taken.
 */
export const findFreeColorSlot = (
  slots: SeriesColorSlots,
): number | undefined => {
  const taken = new Set(Object.values(slots));
  for (let slot = 0; slot < MAX_VISIBLE_SERIES; slot++) {
    if (!taken.has(slot)) {
      return slot;
    }
  }
  return undefined;
};

/** The backend aggregates metrics into one bucket per hour. */
export const BUCKET_INTERVAL_MS = 60 * 60 * 1000;

/**
 * Upper bound on the number of buckets to materialize, as a guard against a
 * stray timestamp far outside the requested window. The chart asks for 72
 * hours, so a month of headroom is generous.
 */
const MAX_BUCKETS = 24 * 30;

/**
 * One row of chart data: every series' value, if any, in a single hourly
 * bucket. A series missing from `values` had no data in that bucket and is
 * plotted as a break in its line rather than as an interpolated point.
 */
export interface TrendsChartRow {
  readonly timestampMs: number;
  readonly values: Readonly<Record<string, number>>;
}

/**
 * Returns the start of the hour containing `isoString`, in epoch ms.
 *
 * Bucketing is done in UTC. Doing it in the viewer's zone would move the
 * boundaries in zones offset by half an hour, splitting a single backend
 * bucket across two rows and inventing gaps that the data does not have.
 */
const bucketOf = (isoString: string): number | undefined => {
  const dt = DateTime.fromISO(isoString, { zone: 'utc' });
  return dt.isValid ? dt.startOf('hour').toMillis() : undefined;
};

/**
 * Reshapes the per-series points returned by the RPC into the row-per-bucket
 * form that recharts consumes.
 *
 * Points are assigned to hourly buckets and rows are emitted for every bucket
 * between the earliest and the latest, including buckets in which no series
 * reported anything. Two properties follow, and both matter:
 *
 *  - Whether a series has a gap is decided by the real one-hour bucket
 *    interval, not by that series' position among the timestamps other series
 *    happen to have reported. A series sampled on its own schedule therefore
 *    still draws a continuous line.
 *  - Rows are evenly spaced in time, so an outage occupies width in proportion
 *    to how long it lasted.
 */
export const buildTrendsChartRows = (
  series: readonly TrendlineSeries[],
): readonly TrendsChartRow[] => {
  const byBucket = new Map<number, Record<string, number>>();
  let earliest = Number.POSITIVE_INFINITY;
  let latest = Number.NEGATIVE_INFINITY;

  for (const s of series) {
    for (const point of s.points) {
      if (!point.timestamp) continue;
      const bucket = bucketOf(point.timestamp);
      if (bucket === undefined) continue;

      const row = byBucket.get(bucket) ?? {};
      // Several points inside one hour should not happen, but if the backend
      // ever emits them the last one wins rather than both being dropped.
      //
      // Negative readings are clamped: the value axis starts at zero, and a
      // negative would stretch it below the labelled range.
      row[s.name] = Math.max(0, point.value);
      byBucket.set(bucket, row);

      earliest = Math.min(earliest, bucket);
      latest = Math.max(latest, bucket);
    }
  }

  if (byBucket.size === 0) return [];

  const bucketCount = (latest - earliest) / BUCKET_INTERVAL_MS + 1;
  if (bucketCount > MAX_BUCKETS) {
    // A timestamp this far outside the window is bad data. Materializing a row
    // per hour across the whole span would exhaust memory, so only the buckets
    // carrying data are emitted. This is a guard against hanging the tab, not
    // an attempt to render the result sensibly: the outlier still sits at its
    // true position on the time axis and squeezes the real data into a sliver.
    return Array.from(byBucket.entries())
      .sort(([a], [b]) => a - b)
      .map(([timestampMs, values]) => ({ timestampMs, values }));
  }

  const rows: TrendsChartRow[] = [];
  for (let ts = earliest; ts <= latest; ts += BUCKET_INTERVAL_MS) {
    rows.push({ timestampMs: ts, values: byBucket.get(ts) ?? {} });
  }
  return rows;
};

/**
 * Picks evenly spaced rows to label on the time axis, always including the
 * first and the last. Appending the last bucket can yield one label more than
 * `maxTicks`, and make the final interval shorter than the rest.
 */
export const buildTimeAxisTicks = (
  rows: readonly TrendsChartRow[],
  maxTicks = 6,
): number[] => {
  if (rows.length === 0) return [];
  if (rows.length <= maxTicks) return rows.map((row) => row.timestampMs);

  const step = Math.max(1, Math.floor((rows.length - 1) / (maxTicks - 1)));
  const ticks: number[] = [];
  for (let i = 0; i < rows.length; i += step) {
    ticks.push(rows[i].timestampMs);
  }

  const last = rows[rows.length - 1].timestampMs;
  if (ticks[ticks.length - 1] !== last) {
    ticks.push(last);
  }
  return ticks;
};

/**
 * The upper bound of the value axis. It is at least 100%, and grows when
 * surplus availability pushes a series above it.
 */
export const computeMaxYScale = (
  series: readonly TrendlineSeries[],
): number => {
  let max = 1.0;
  for (const s of series) {
    for (const point of s.points) {
      if (point.value > max) {
        max = point.value;
      }
    }
  }
  return Math.max(1.0, Math.ceil(max * 5) / 5);
};

/** Six evenly spaced value-axis ticks, from zero to `maxYScale`. */
export const buildValueAxisTicks = (maxYScale: number): number[] =>
  [0, 0.2, 0.4, 0.6, 0.8, 1.0].map((step) => +(step * maxYScale).toFixed(2));

export const formatTickLabel = (timestampMs: number) => {
  const dt = DateTime.fromMillis(timestampMs).setZone('UTC-7');
  return `${dt.toFormat('M/d HH:mm')} GMT-7`;
};

export const formatTooltipDate = (timestampMs: number) => {
  const dt = DateTime.fromMillis(timestampMs).setZone('UTC-7');
  return `${dt.toFormat('ccc, dd LLL yyyy, HH:mm')} GMT-7`;
};

export const formatPercentTick = (value: number) =>
  `${Math.round(value * 100)}%`;
