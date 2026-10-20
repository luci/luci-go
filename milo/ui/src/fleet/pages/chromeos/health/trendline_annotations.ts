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

import { BUCKET_INTERVAL_MS } from './trends_chart_data';

/**
 * Represents an annotation / timeline event plotted on historical trendlines
 * (e.g., algorithm shifts, status counting changes, metric updates).
 */
export interface TrendlineAnnotation {
  /**
   * Timestamp of the event, represented as a Luxon DateTime.
   */
  readonly timestamp: DateTime<boolean>;
  /** Short title describing the event. */
  readonly title: string;
  /** Detailed description of what changed and its expected impact on metrics. */
  readonly description: string;
}

export type TrendlineEvent = TrendlineAnnotation;

/**
 * Single Source of Truth (SSOT) for trendline annotations and algorithm change markers.
 * Add new timeline events here to automatically display vertical event markers and tooltips
 * on historical availability and health trendlines.
 */
export const TRENDLINE_ANNOTATIONS: readonly TrendlineAnnotation[] = [
  {
    // 2026-10-20 20:00 CEST (Europe/Warsaw, UTC+2) -> 18:00 UTC
    timestamp: DateTime.fromISO('2026-10-20T18:00:00Z', { zone: 'utc' }),
    title: 'Algorithm change',
    description:
      'Health calculation updated to include additional device statuses. An increase in health scores is expected as a result.',
  },
];

export const TRENDLINE_EVENTS = TRENDLINE_ANNOTATIONS;

/**
 * Converts an annotation's DateTime timestamp to epoch milliseconds.
 * Returns undefined if the timestamp is missing or invalid.
 */
export const getAnnotationTimestampMs = (
  annotation: TrendlineAnnotation,
): number | undefined => {
  return annotation.timestamp.isValid
    ? annotation.timestamp.toMillis()
    : undefined;
};

/**
 * Returns all annotations that fall within the specified time range [startMs, endMs] (inclusive).
 */
export const getAnnotationsInRange = (
  annotations: readonly TrendlineAnnotation[],
  startMs: number,
  endMs: number,
): readonly TrendlineAnnotation[] => {
  return annotations.filter((annotation) => {
    const ts = getAnnotationTimestampMs(annotation);
    return ts !== undefined && ts >= startMs && ts <= endMs;
  });
};

/**
 * Finds the first annotation whose timestamp falls within the hourly bucket starting at bucketMs.
 */
export const findAnnotationForBucket = (
  annotations: readonly TrendlineAnnotation[],
  bucketMs: number,
  intervalMs = BUCKET_INTERVAL_MS,
): TrendlineAnnotation | undefined => {
  return annotations.find((annotation) => {
    const ts = getAnnotationTimestampMs(annotation);
    return ts !== undefined && ts >= bucketMs && ts < bucketMs + intervalMs;
  });
};
