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
  PerfChartSeries_MatchType,
  perfChartSeries_MatchTypeFromJSON,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Returns the match type to use for a series, defaulting to EXACT.
 *
 * Dashboards fetched from the API carry enums as their JSON string names
 * (e.g. "REGEX"), so string values are converted to the numeric enum. Without
 * this, the Match Type select shows nothing for saved RegEx/Wildcard series.
 */
export function getEffectiveMatchType(
  matchType?: PerfChartSeries_MatchType | string | null,
): PerfChartSeries_MatchType {
  const numericMatchType =
    typeof matchType === 'string'
      ? perfChartSeries_MatchTypeFromJSON(matchType)
      : matchType;
  if (
    numericMatchType === null ||
    numericMatchType === undefined ||
    numericMatchType === PerfChartSeries_MatchType.MATCH_TYPE_UNSPECIFIED
  ) {
    return PerfChartSeries_MatchType.EXACT;
  }
  return numericMatchType;
}

/**
 * Whether a series metric field is a pattern (RegEx or Wildcard) that the
 * server expands into one series per matching metric key.
 */
export function isPatternMatchType(
  matchType: PerfChartSeries_MatchType,
): boolean {
  return (
    matchType === PerfChartSeries_MatchType.REGEX ||
    matchType === PerfChartSeries_MatchType.WILDCARD
  );
}
