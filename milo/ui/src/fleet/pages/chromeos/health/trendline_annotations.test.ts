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

import {
  findAnnotationForBucket,
  getAnnotationsInRange,
  getAnnotationTimestampMs,
  TRENDLINE_ANNOTATIONS,
  TrendlineAnnotation,
} from './trendline_annotations';

describe('trendline_annotations SSOT', () => {
  it('contains valid default annotations with non-empty fields and valid DateTime timestamps', () => {
    expect(TRENDLINE_ANNOTATIONS.length).toBeGreaterThan(0);
    for (const annotation of TRENDLINE_ANNOTATIONS) {
      expect(annotation.title).toBeTruthy();
      expect(annotation.description).toBeTruthy();
      expect(annotation.timestamp).toBeInstanceOf(DateTime);
      expect(annotation.timestamp.isValid).toBe(true);
      const ts = getAnnotationTimestampMs(annotation);
      expect(ts).toBeDefined();
      expect(Number.isFinite(ts)).toBe(true);
    }
  });

  it('includes the 2026-10-20 algorithm change annotation', () => {
    const change = TRENDLINE_ANNOTATIONS.find(
      (a) => a.title === 'Algorithm change',
    );
    expect(change).toBeDefined();
    expect(change?.description).toContain('additional device statuses');
    expect(change?.timestamp.toISO()).toContain('2026-10-20T18:00:00');
  });

  describe('getAnnotationTimestampMs', () => {
    it('returns epoch milliseconds for valid DateTime', () => {
      const dt = DateTime.fromISO('2026-10-20T12:00:00Z', { zone: 'utc' });
      const annotation: TrendlineAnnotation = {
        timestamp: dt,
        title: 'Test',
        description: 'Test description',
      };
      expect(getAnnotationTimestampMs(annotation)).toBe(dt.toMillis());
    });

    it('returns undefined for invalid DateTime', () => {
      const annotation: TrendlineAnnotation = {
        timestamp: DateTime.invalid('Invalid date'),
        title: 'Test',
        description: 'Test description',
      };
      expect(getAnnotationTimestampMs(annotation)).toBeUndefined();
    });
  });

  describe('getAnnotationsInRange', () => {
    const a1: TrendlineAnnotation = {
      timestamp: DateTime.fromISO('2026-10-20T10:00:00Z', { zone: 'utc' }),
      title: 'A1',
      description: 'Desc 1',
    };
    const a2: TrendlineAnnotation = {
      timestamp: DateTime.fromISO('2026-10-20T15:00:00Z', { zone: 'utc' }),
      title: 'A2',
      description: 'Desc 2',
    };
    const a3: TrendlineAnnotation = {
      timestamp: DateTime.fromISO('2026-10-20T20:00:00Z', { zone: 'utc' }),
      title: 'A3',
      description: 'Desc 3',
    };
    const annotations = [a1, a2, a3];

    it('returns only annotations within the given range', () => {
      const startMs = Date.parse('2026-10-20T12:00:00Z');
      const endMs = Date.parse('2026-10-20T18:00:00Z');
      const inRange = getAnnotationsInRange(annotations, startMs, endMs);
      expect(inRange).toEqual([a2]);
    });

    it('includes boundary timestamps', () => {
      const startMs = Date.parse('2026-10-20T10:00:00Z');
      const endMs = Date.parse('2026-10-20T15:00:00Z');
      const inRange = getAnnotationsInRange(annotations, startMs, endMs);
      expect(inRange).toEqual([a1, a2]);
    });

    it('returns empty array when no annotations fall within range', () => {
      const startMs = Date.parse('2026-10-21T00:00:00Z');
      const endMs = Date.parse('2026-10-21T10:00:00Z');
      expect(getAnnotationsInRange(annotations, startMs, endMs)).toEqual([]);
    });

    it('ignores annotations with invalid timestamps', () => {
      const invalid: TrendlineAnnotation = {
        timestamp: DateTime.invalid('bad date'),
        title: 'Bad',
        description: 'Bad desc',
      };
      const result = getAnnotationsInRange(
        [invalid, a1],
        Date.parse('2026-10-20T00:00:00Z'),
        Date.parse('2026-10-20T23:59:59Z'),
      );
      expect(result).toEqual([a1]);
    });
  });

  describe('findAnnotationForBucket', () => {
    const a1: TrendlineAnnotation = {
      timestamp: DateTime.fromISO('2026-10-20T10:15:00Z', { zone: 'utc' }),
      title: 'A1',
      description: 'Desc 1',
    };
    const annotations = [a1];

    it('finds annotation falling within the hourly bucket', () => {
      const bucketMs = Date.parse('2026-10-20T10:00:00Z');
      expect(findAnnotationForBucket(annotations, bucketMs)).toEqual(a1);
    });

    it('returns undefined if bucket does not cover the timestamp', () => {
      const prevBucketMs = Date.parse('2026-10-20T09:00:00Z');
      const nextBucketMs = Date.parse('2026-10-20T11:00:00Z');
      expect(
        findAnnotationForBucket(annotations, prevBucketMs),
      ).toBeUndefined();
      expect(
        findAnnotationForBucket(annotations, nextBucketMs),
      ).toBeUndefined();
    });
  });
});
