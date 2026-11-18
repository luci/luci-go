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

import { PerfChartSeries_MatchType } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { getEffectiveMatchType, isPatternMatchType } from './match_type_utils';

describe('getEffectiveMatchType', () => {
  it('defaults missing and unspecified match types to EXACT', () => {
    expect(getEffectiveMatchType(undefined)).toBe(
      PerfChartSeries_MatchType.EXACT,
    );
    expect(getEffectiveMatchType(null)).toBe(PerfChartSeries_MatchType.EXACT);
    expect(
      getEffectiveMatchType(PerfChartSeries_MatchType.MATCH_TYPE_UNSPECIFIED),
    ).toBe(PerfChartSeries_MatchType.EXACT);
    expect(getEffectiveMatchType('MATCH_TYPE_UNSPECIFIED')).toBe(
      PerfChartSeries_MatchType.EXACT,
    );
  });

  it('converts JSON enum names from the API to the numeric enum', () => {
    expect(getEffectiveMatchType('REGEX')).toBe(
      PerfChartSeries_MatchType.REGEX,
    );
    expect(getEffectiveMatchType('WILDCARD')).toBe(
      PerfChartSeries_MatchType.WILDCARD,
    );
    expect(getEffectiveMatchType('EXACT')).toBe(
      PerfChartSeries_MatchType.EXACT,
    );
  });

  it('keeps numeric match types', () => {
    expect(getEffectiveMatchType(PerfChartSeries_MatchType.REGEX)).toBe(
      PerfChartSeries_MatchType.REGEX,
    );
  });
});

describe('isPatternMatchType', () => {
  it('is true only for RegEx and Wildcard', () => {
    expect(isPatternMatchType(PerfChartSeries_MatchType.REGEX)).toBe(true);
    expect(isPatternMatchType(PerfChartSeries_MatchType.WILDCARD)).toBe(true);
    expect(isPatternMatchType(PerfChartSeries_MatchType.EXACT)).toBe(false);
    expect(
      isPatternMatchType(PerfChartSeries_MatchType.MATCH_TYPE_UNSPECIFIED),
    ).toBe(false);
  });
});
