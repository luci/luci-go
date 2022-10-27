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

import {
  ClusterExoneratedTestVariant,
} from '@/services/cluster';
import {
  TestVariantFailureRateAnalysis,
  TestVariantFailureRateAnalysisVerdictExample,
  TestVariantFailureRateAnalysisRecentVerdict,
} from '@/services/test_variants';
import { Variant } from '@/services/shared_models';

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
  recentVerdictsWithUnexpectedRuns: number;
  runFlakyVerdictExamples: TestVariantFailureRateAnalysisVerdictExample[];
  recentVerdicts: TestVariantFailureRateAnalysisRecentVerdict[];
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

export const isFlakyCriteriaAlmostMet = (tv: ExoneratedTestVariant): boolean => {
  return !isFlakyCriteriaMet(tv) && tv.runFlakyVerdicts5wd >= 2;
};

export const isFlakyCriteriaMet = (tv: ExoneratedTestVariant): boolean => {
  return tv.runFlakyVerdicts1wd >= 1 && tv.runFlakyVerdicts5wd >= 3;
};

export const isFailureCriteriaAlmostMet = (tv: ExoneratedTestVariant): boolean => {
  return !isFailureCriteriaMet(tv) && tv.recentVerdictsWithUnexpectedRuns >= 4;
};

export const isFailureCriteriaMet = (tv: ExoneratedTestVariant): boolean => {
  return tv.recentVerdictsWithUnexpectedRuns >= 7;
};

export const testVariantFromAnalysis = (etv: ClusterExoneratedTestVariant, atv: TestVariantFailureRateAnalysis): ExoneratedTestVariant => {
  // It would be better to also check the variant matches, but this is mostly
  // to detect programming errors.
  if (atv.testId != etv.testId) {
    throw new Error('exonerated test variant and analysed test variant do not match');
  }
  let runFlakyVerdicts1wd = 0;
  let runFlakyVerdicts5wd = 0;
  atv.intervalStats.forEach((interval) => {
    if (interval.intervalAge == 1) {
      runFlakyVerdicts1wd = interval.totalRunFlakyVerdicts || 0;
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
    lastExoneration: etv.lastExoneration,
    criticalFailuresExonerated: etv.criticalFailuresExonerated,
    runFlakyVerdicts1wd: runFlakyVerdicts1wd,
    runFlakyVerdicts5wd: runFlakyVerdicts5wd,
    recentVerdictsWithUnexpectedRuns: recentVerdictsWithUnexpectedRuns,
    recentVerdicts: atv.recentVerdicts || [],
    runFlakyVerdictExamples: atv.runFlakyVerdictExamples || [],
  };
};

// exonerationLevel returns whether the test variant is being exonerated as
// a number which can be sorted.
export const exonerationLevel = (tv :ExoneratedTestVariant): number => {
  if (isFlakyCriteriaMet(tv) || isFailureCriteriaMet(tv)) {
    return 2;
  } else if (isFlakyCriteriaAlmostMet(tv) || isFailureCriteriaAlmostMet(tv)) {
    return 1;
  } else {
    return 0;
  }
};

export const sortTestVariants = (tvs :ExoneratedTestVariant[], field: SortableField, ascending: boolean): ExoneratedTestVariant[] => {
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
        return exonerationLevel(a) - exonerationLevel(b);
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
