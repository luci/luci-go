// Copyright 2022 The LUCI Authors.
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

import dayjs from 'dayjs';

import { Variant } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  ClusterExoneratedTestVariant,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  TestVariantFailureRateAnalysis,
  TestVariantFailureRateAnalysis_VerdictExample,
  TestVariantFailureRateAnalysis_RecentVerdict,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

// Fields that can be used for sorting FailureGroups.
export type SortableField = 'testId' | 'beingExonerated' | 'lastExoneration' | 'criticalFailuresExonerated';

export interface ExoneratedTestVariant {
  // A unique key for the test variant.
  key: string;
  testId: string;
  variant?: Variant;
  // RFC 3339 encoded date/time of the last exoneration.
  lastExoneration: string;
  // The number of critical failures exonerated in the last week.
  criticalFailuresExonerated: number;
  runFlakyVerdicts1wd: number;
  runFlakyVerdicts5wd: number;
  runFlakyPercentage1wd: number;
  recentVerdictsWithUnexpectedRuns: number;
  runFlakyVerdictExamples: readonly TestVariantFailureRateAnalysis_VerdictExample[];
  recentVerdicts: readonly TestVariantFailureRateAnalysis_RecentVerdict[];
}

const testVariantKey = (testId: string, variant?: Variant): string => {
  if (!variant) {
    variant = { def: {} };
  }
  const keyValues: string[] = [];
  for (const key in variant.def) {
    if (!Object.prototype.hasOwnProperty.call(variant.def, key)) {
      continue;
    }
    const value = variant.def[key] || '';
    keyValues.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
  }
  keyValues.sort();
  return encodeURI(testId) + '/' + keyValues.join('/');
};

export interface ExonerationCriteria {
  runFlakyVerdicts1wd: number;
  runFlakyVerdicts5wd: number;
  recentVerdictsWithUnexpectedRuns: number;
  runFlakyPercentage1wd: number;
}
export const ChromiumCriteria: ExonerationCriteria = {
  runFlakyVerdicts1wd: 1,
  runFlakyVerdicts5wd: 3,
  recentVerdictsWithUnexpectedRuns: 7,
  runFlakyPercentage1wd: 0,
};

export const ChromeOSCriteria: ExonerationCriteria = {
  runFlakyVerdicts1wd: 0,
  runFlakyVerdicts5wd: 3,
  recentVerdictsWithUnexpectedRuns: 6,
  runFlakyPercentage1wd: 1,
};

export const isFlakyCriteriaAlmostMet = (criteria: ExonerationCriteria, tv: ExoneratedTestVariant): boolean => {
  if (isFlakyCriteriaMet(criteria, tv)) {
    return false;
  }
  if (criteria.runFlakyVerdicts5wd > 0 && tv.runFlakyVerdicts5wd >= Math.ceil(criteria.runFlakyVerdicts5wd / 2)) {
    return true;
  }
  if (criteria.runFlakyVerdicts1wd > 0 && tv.runFlakyVerdicts1wd >= Math.ceil(criteria.runFlakyVerdicts1wd / 2)) {
    return true;
  }
  return false;
};

export const isFlakyCriteriaMet = (criteria: ExonerationCriteria, tv: ExoneratedTestVariant): boolean => {
  if (criteria.runFlakyVerdicts1wd > 0 && tv.runFlakyVerdicts1wd < criteria.runFlakyVerdicts1wd) {
    return false;
  }
  if (criteria.runFlakyVerdicts5wd > 0 && tv.runFlakyVerdicts5wd < criteria.runFlakyVerdicts5wd) {
    return false;
  }
  return true;
};

export const isFailureCriteriaAlmostMet = (criteria: ExonerationCriteria, tv: ExoneratedTestVariant): boolean => {
  if (isFlakyCriteriaMet(criteria, tv)) {
    return false;
  }
  if (criteria.recentVerdictsWithUnexpectedRuns > 1 && tv.recentVerdictsWithUnexpectedRuns >= Math.ceil(criteria.recentVerdictsWithUnexpectedRuns / 2)) {
    return true;
  }
  return false;
};

export const isFailureCriteriaMet = (criteria: ExonerationCriteria, tv: ExoneratedTestVariant): boolean => {
  if (criteria.recentVerdictsWithUnexpectedRuns > 0 && tv.recentVerdictsWithUnexpectedRuns < criteria.recentVerdictsWithUnexpectedRuns) {
    return false;
  }
  return true;
};

export const testVariantFromAnalysis = (etv: ClusterExoneratedTestVariant, atv: TestVariantFailureRateAnalysis): ExoneratedTestVariant => {
  // It would be better to also check the variant matches, but this is mostly
  // to detect programming errors.
  if (atv.testId != etv.testId) {
    throw new Error('exonerated test variant and analysed test variant do not match');
  }
  let runFlakyVerdicts1wd = 0;
  let runFlakyPercentage1wd = 0;
  let runFlakyVerdicts5wd = 0;
  atv.intervalStats.forEach((interval) => {
    if (interval.intervalAge == 1) {
      runFlakyVerdicts1wd = interval.totalRunFlakyVerdicts || 0;
      const totalVerdicts = (interval.totalRunFlakyVerdicts || 0) + (interval.totalRunExpectedVerdicts || 0) + (interval.totalRunUnexpectedVerdicts || 0);
      runFlakyPercentage1wd = totalVerdicts == 0 ? 0 : Math.round((100*(interval.totalRunFlakyVerdicts || 0))/totalVerdicts);
    }
    if (interval.intervalAge >= 1 && interval.intervalAge <= 5) {
      runFlakyVerdicts5wd += interval.totalRunFlakyVerdicts || 0;
    }
  });
  let recentVerdictsWithUnexpectedRuns = 0;
  atv.recentVerdicts?.forEach((v) => {
    if (v.hasUnexpectedRuns) {
      recentVerdictsWithUnexpectedRuns++;
    }
  });

  return {
    key: testVariantKey(atv.testId, atv.variant),
    testId: atv.testId,
    variant: atv.variant,
    lastExoneration: etv.lastExoneration || '',
    criticalFailuresExonerated: etv.criticalFailuresExonerated,
    runFlakyVerdicts1wd: runFlakyVerdicts1wd,
    runFlakyVerdicts5wd: runFlakyVerdicts5wd,
    runFlakyPercentage1wd: runFlakyPercentage1wd,
    recentVerdictsWithUnexpectedRuns: recentVerdictsWithUnexpectedRuns,
    recentVerdicts: atv.recentVerdicts || [],
    runFlakyVerdictExamples: atv.runFlakyVerdictExamples || [],
  };
};

// exonerationLevel returns whether the test variant is being exonerated as
// a number which can be sorted.
export const exonerationLevel = (criteria: ExonerationCriteria, tv: ExoneratedTestVariant): number => {
  if (isFlakyCriteriaMet(criteria, tv) || isFailureCriteriaMet(criteria, tv)) {
    return 2;
  } else if (isFlakyCriteriaAlmostMet(criteria, tv) || isFailureCriteriaAlmostMet(criteria, tv)) {
    return 1;
  } else {
    return 0;
  }
};

export const sortTestVariants = (criteria: ExonerationCriteria, tvs: ExoneratedTestVariant[], field: SortableField, ascending: boolean): ExoneratedTestVariant[] => {
  const cloneTVs = [...tvs];
  const compare = (a: ExoneratedTestVariant, b: ExoneratedTestVariant): number => {
    switch (field) {
      case 'testId':
        if (a.testId > b.testId) {
          return 1;
        } else if (b.testId > a.testId) {
          return -1;
        }
        return 0;
      case 'beingExonerated':
        return exonerationLevel(criteria, a) - exonerationLevel(criteria, b);
      case 'criticalFailuresExonerated':
        return a.criticalFailuresExonerated - b.criticalFailuresExonerated;
      case 'lastExoneration':
        return dayjs(a.lastExoneration).unix() - dayjs(b.lastExoneration).unix();
      default:
        throw new Error('unknown field: ' + field);
    }
  };
  cloneTVs.sort((a, b) => ascending ? compare(a, b) : compare(b, a));
  return cloneTVs;
};
