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

import { SemanticStatusType } from '@/common/styles/status_styles';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { Segment_Counts } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

export function getFormattedFailureRateFromSegmentCount(
  counts?: Segment_Counts | null,
): string {
  if (!counts) {
    return 'N/A';
  }
  const { unexpectedResults = 0, totalResults = 0 } = counts;
  const rate = totalResults > 0 ? unexpectedResults / totalResults : 0;
  return rate.toLocaleString(undefined, {
    style: 'percent',
    minimumFractionDigits: 0,
    maximumFractionDigits: 1,
  });
}

export function getFailureRateStatusTypeFromSegmentCount(
  counts?: Segment_Counts | null,
): SemanticStatusType {
  if (!counts) {
    return 'unknown';
  }
  const { unexpectedResults = 0, totalResults = 0 } = counts;
  const ratePercent =
    totalResults > 0 ? (unexpectedResults / totalResults) * 100 : 0;
  return determineRateStatusType(ratePercent);
}

function determineRateStatusType(ratePercent: number): SemanticStatusType {
  if (ratePercent <= 5) return 'passed';
  if (ratePercent > 5 && ratePercent < 95) return 'flaky';
  if (ratePercent >= 95) return 'failed';
  return 'unknown';
}

/**
 * Given an ISO string, returns a human-readable string representing how long
 * ago that time was. Handles details specific to history segment display.
 *
 * @param isoString The ISO string to format.
 * @param nowDateTime The current time. Defaults to `DateTime.now()`.
 */
export function formatSegmentTimestamp(
  isoString?: string,
  nowDateTime: DateTime = DateTime.now(),
): string | undefined {
  if (!isoString) return undefined;
  const pastDateTime = DateTime.fromISO(isoString, { zone: 'utc' });
  if (!pastDateTime.isValid) return undefined;
  const approxDurationText = displayApproxDuration(
    nowDateTime.diff(pastDateTime),
  );
  return approxDurationText && approxDurationText !== 'N/A'
    ? `${approxDurationText} ago`
    : undefined;
}
