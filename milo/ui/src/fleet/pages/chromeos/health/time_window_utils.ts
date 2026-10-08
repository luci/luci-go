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

const MS_IN_HOUR = 60 * 60 * 1000;

/**
 * Number of hours to fetch for single-point status cards (Hero and HealthSliceItem).
 * Provides a 2-hour buffer across the current and preceding hour boundary to guarantee
 * at least one calculated point even during top-of-the-hour ingestion transitions.
 */
export const RECENT_HOURS_WINDOW = 2;

/**
 * Number of hours to fetch for PerformanceRankingCard.
 * Covers 24 hours of operational delta plus a 2-hour buffer for hour-boundary alignment.
 */
export const RANKING_HOURS_WINDOW = 26;

/**
 * Returns an ISO timestamp window rounded down to the start of the current hour.
 *
 * Truncating to the full hour aligns with backend metric_hour bucketing and
 * ensures referential query key stability across concurrently mounting
 * dashboard components (preventing redundant pRPC queries due to sub-second
 * timestamp drift).
 *
 * @param hoursBack Number of hours prior to the current hour boundary for startTime.
 * @param nowMs Reference epoch time in ms (defaults to Date.now()).
 */
export const getHourAlignedTimeWindow = (
  hoursBack: number,
  nowMs: number = Date.now(),
): { startTime: string; endTime: string } => {
  const currentHourMs = Math.floor(nowMs / MS_IN_HOUR) * MS_IN_HOUR;
  return {
    startTime: new Date(currentHourMs - hoursBack * MS_IN_HOUR).toISOString(),
    endTime: new Date(currentHourMs).toISOString(),
  };
};
