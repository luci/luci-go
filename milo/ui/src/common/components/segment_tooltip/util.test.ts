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

import { Segment_Counts } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import {
  formatSegmentTimestamp,
  getFailureRateStatusTypeFromSegmentCount,
  getFormattedFailureRateFromSegmentCount,
} from './util';

describe('segment_tooltip util', () => {
  describe('getFormattedFailureRateFromSegmentCount', () => {
    it('returns N/A when counts is null or undefined', () => {
      expect(getFormattedFailureRateFromSegmentCount(null)).toBe('N/A');
      expect(getFormattedFailureRateFromSegmentCount(undefined)).toBe('N/A');
    });

    it('formats failure rate as percentage from unexpectedResults/totalResults', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 10,
        totalResults: 100,
      });
      expect(getFormattedFailureRateFromSegmentCount(counts)).toBe('10%');
    });

    it('formats fractional percentages with up to 1 decimal digit', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 1,
        totalResults: 3,
      });
      expect(getFormattedFailureRateFromSegmentCount(counts)).toBe('33.3%');
    });

    it('handles zero results gracefully', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 0,
        totalResults: 0,
      });
      expect(getFormattedFailureRateFromSegmentCount(counts)).toBe('0%');
    });
  });

  describe('getFailureRateStatusTypeFromSegmentCount', () => {
    it('returns unknown when counts is null or undefined', () => {
      expect(getFailureRateStatusTypeFromSegmentCount(null)).toBe('unknown');
    });

    it('returns passed for <= 5% failure rate', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 5,
        totalResults: 100,
      });
      expect(getFailureRateStatusTypeFromSegmentCount(counts)).toBe('passed');
    });

    it('returns flaky for > 5% and < 95% failure rate', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 50,
        totalResults: 100,
      });
      expect(getFailureRateStatusTypeFromSegmentCount(counts)).toBe('flaky');
    });

    it('returns failed for >= 95% failure rate', () => {
      const counts = Segment_Counts.fromPartial({
        unexpectedResults: 95,
        totalResults: 100,
      });
      expect(getFailureRateStatusTypeFromSegmentCount(counts)).toBe('failed');
    });
  });

  describe('formatSegmentTimestamp', () => {
    const now = DateTime.fromISO('2026-08-01T12:00:00Z');

    it('returns undefined for empty or invalid isoString', () => {
      expect(formatSegmentTimestamp(undefined, now)).toBeUndefined();
      expect(formatSegmentTimestamp('', now)).toBeUndefined();
      expect(formatSegmentTimestamp('not-a-date', now)).toBeUndefined();
    });

    it('formats past timestamp correctly', () => {
      const twoHoursAgo = '2026-08-01T10:00:00Z';
      expect(formatSegmentTimestamp(twoHoursAgo, now)).toBe('2 hours ago');
    });
  });
});
