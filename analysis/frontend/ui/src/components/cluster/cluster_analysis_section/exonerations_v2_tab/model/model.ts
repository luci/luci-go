// Copyright 2024 The LUCI Authors.
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

// import { Variant } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  TestStabilityCriteria,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import { ExoneratedTestVariantBranch, ExoneratedTestVariantBranches } from '@/hooks/use_fetch_exonerated_test_variant_branches';
import { SourceRef } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';

// Fields that can be used for sorting FailureGroups.
export type SortableField = 'testId' | 'sourceRef' | 'beingExonerated' | 'lastExoneration' | 'criticalFailuresExonerated';

export enum CriteriaMetIndicator {
  NotMet = 1,
  AlmostMet,
  Met
}

const isFlakyCriteriaAlmostMet = (criteria: TestStabilityCriteria, tvb: ExoneratedTestVariantBranch): boolean => {
  if (isFlakyCriteriaMet(tvb)) {
    return false;
  }
  if (!criteria.flakeRate) {
    return false;
  }
  if (tvb.flakeRate.totalVerdicts == 0) {
    return false;
  }
  const runFlakyRate = tvb.flakeRate.runFlakyVerdicts / tvb.flakeRate.totalVerdicts;
  return (runFlakyRate >= (criteria.flakeRate.flakeRateThreshold / 2.0) && tvb.flakeRate.runFlakyVerdicts >= (criteria.flakeRate.flakeThreshold / 2.0));
};

const isFlakyCriteriaMet = (tvb: ExoneratedTestVariantBranch): boolean => {
  return tvb.flakeRate.isMet;
};

const isFailureCriteriaAlmostMet = (criteria: TestStabilityCriteria, tvb: ExoneratedTestVariantBranch): boolean => {
  if (isFailureCriteriaMet(tvb)) {
    return false;
  }
  if (!criteria.failureRate) {
    return false;
  }
  return (tvb.failureRate.consecutiveUnexpectedTestRuns > 0 && tvb.failureRate.consecutiveUnexpectedTestRuns >= (criteria.failureRate.consecutiveFailureThreshold / 2)) ||
    (tvb.failureRate.unexpectedTestRuns > 0 && tvb.failureRate.unexpectedTestRuns >= (criteria.failureRate.failureThreshold / 2));
};

const isFailureCriteriaMet = (tvb: ExoneratedTestVariantBranch): boolean => {
  return tvb.failureRate.isMet;
};

export const flakyCriteriaMetIndicator = (criteria: TestStabilityCriteria, tvb: ExoneratedTestVariantBranch) => {
  if (isFlakyCriteriaMet(tvb)) {
    return CriteriaMetIndicator.Met;
  } else if (isFlakyCriteriaAlmostMet(criteria, tvb)) {
    return CriteriaMetIndicator.AlmostMet;
  }
  return CriteriaMetIndicator.NotMet;
};

export const failureCriteriaMetIndicator = (criteria: TestStabilityCriteria, tvb: ExoneratedTestVariantBranch) => {
  if (isFailureCriteriaMet(tvb)) {
    return CriteriaMetIndicator.Met;
  } else if (isFailureCriteriaAlmostMet(criteria, tvb)) {
    return CriteriaMetIndicator.AlmostMet;
  }
  return CriteriaMetIndicator.NotMet;
};

// AnyCriteriaMetIndicator returns whether the test variant is
// meeting, or close to meeting, any exoneration criteria.
export const anyCriteriaMetIndicator = (criteria: TestStabilityCriteria, tvb: ExoneratedTestVariantBranch): CriteriaMetIndicator => {
  if (isFlakyCriteriaMet(tvb) || isFailureCriteriaMet(tvb)) {
    return CriteriaMetIndicator.Met;
  } else if (isFlakyCriteriaAlmostMet(criteria, tvb) || isFailureCriteriaAlmostMet(criteria, tvb)) {
    return CriteriaMetIndicator.AlmostMet;
  }
  return CriteriaMetIndicator.NotMet;
};

export const sortTestVariantBranches = (response: ExoneratedTestVariantBranches, field: SortableField, ascending: boolean): ExoneratedTestVariantBranch[] => {
  const cloneTVBs = [...response.testVariantBranches];
  const compare = (a: ExoneratedTestVariantBranch, b: ExoneratedTestVariantBranch): number => {
    let result = 0;
    switch (field) {
      case 'testId':
        result = compareTestIds(a.testId, b.testId);
        break;
      case 'sourceRef':
        result = compareSourceRefs(a.sourceRef, b.sourceRef);
        break;
      case 'beingExonerated':
        result = anyCriteriaMetIndicator(response.criteria, a) - anyCriteriaMetIndicator(response.criteria, b);
        break;
      case 'criticalFailuresExonerated':
        result = a.criticalFailuresExonerated - b.criticalFailuresExonerated;
        break;
      case 'lastExoneration':
        result = dayjs(a.lastExoneration).unix() - dayjs(b.lastExoneration).unix();
        break;
      default:
        throw new Error('unknown field: ' + field);
    }
    // Try secondary sort orders.
    if (result == 0) {
      result = compareTestIds(a.testId, b.testId);
    }
    if (result == 0) {
      result = compareSourceRefs(a.sourceRef, b.sourceRef);
    }
    return result;
  };
  cloneTVBs.sort((a, b) => ascending ? compare(a, b) : compare(b, a));
  return cloneTVBs;
};

const compareSourceRefs = (a: SourceRef, b: SourceRef) : number => {
  if (a.gitiles === undefined || b.gitiles === undefined) {
    if (a.gitiles !== undefined) {
      return 1;
    } else if (b.gitiles !== undefined) {
      return -1;
    }
    // Both a and b have empty source refs.
    return 0;
  }
  if (a.gitiles.host != b.gitiles.host) {
    return (a.gitiles.host > b.gitiles.host) ? 1 : -1;
  }
  if (a.gitiles.project != b.gitiles.project) {
    return (a.gitiles.project > b.gitiles.project) ? 1 : -1;
  }
  if (a.gitiles.ref != b.gitiles.ref) {
    return (a.gitiles.ref > b.gitiles.ref) ? 1 : -1;
  }
  return 0;
};

const compareTestIds = (a: string, b: string) : number => {
  if (a != b) {
    return (a > b) ? 1 : -1;
  }
  return 0;
};
