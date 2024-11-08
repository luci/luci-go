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

import { DateTime } from 'luxon';
import { nanoid } from 'nanoid';

import {
  DistinctClusterFailure,
  DistinctClusterFailure_Exoneration,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  BuildStatus,
  Variant,
  ExonerationReason,
  PresubmitRunMode,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

import { unionVariant } from './variant_tools';

/**
 * Creates a list of distinct variants found in the list of failures provided.
 *
 * @param {DistinctClusterFailure[]} failures the failures list.
 * @return {VariantGroup[]} A list of distinct variants.
 */
export const countDistictVariantValues = (
  failures: DistinctClusterFailure[],
): VariantGroup[] => {
  if (!failures) {
    return [];
  }
  const variantGroups: VariantGroup[] = [];
  failures.forEach((failure) => {
    if (failure.variant === undefined) {
      return;
    }
    const def = failure.variant.def;
    for (const key in def) {
      if (!Object.prototype.hasOwnProperty.call(def, key)) {
        continue;
      }
      const value = def[key] || '';
      const variant = variantGroups.filter((e) => e.key === key)?.[0];
      if (!variant) {
        variantGroups.push({ key: key, values: [value] });
      } else {
        if (variant.values.indexOf(value) === -1) {
          variant.values.push(value);
        }
      }
    }
  });
  return variantGroups;
};

// group a number of failures into a tree of failure groups.
// grouper is a function that returns a list of keys, one corresponding to each level of the grouping tree.
// impactFilter controls how metric counts are aggregated from failures into parent groups (see treeCounts and rejected... functions).
export const groupFailures = (
  failures: DistinctClusterFailure[],
  grouper: (f: DistinctClusterFailure) => GroupKey[],
): FailureGroup[] => {
  const topGroups: FailureGroup[] = [];
  const leafKey: GroupKey = { type: 'leaf', value: '' };
  failures.forEach((f) => {
    const keys = grouper(f);
    let groups = topGroups;
    const failureTime = DateTime.fromISO(f.partitionTime || '');
    let level = 0;
    for (const key of keys) {
      const group = getOrCreateGroup(groups, key, failureTime.toISO());
      group.level = level;
      level += 1;
      groups = group.children;
    }
    const failureGroup = newGroup(leafKey, failureTime.toISO());
    failureGroup.failure = f;
    failureGroup.level = level;
    groups.push(failureGroup);
  });
  for (const group of topGroups) {
    populateCommonVariant(group);
  }
  return topGroups;
};

const populateCommonVariant = (group: FailureGroup): Variant | undefined => {
  if (group.failure) {
    return group.failure.variant;
  }
  let firstChild = true;
  let commonVariant: Variant | undefined;
  for (const child of group.children) {
    if (firstChild) {
      // For the first child, take the variant as the common variant.
      commonVariant = populateCommonVariant(child);
      firstChild = false;
    } else {
      // For subsequent children, combine the variant.
      const childVariant = populateCommonVariant(child);
      commonVariant = unionVariant(commonVariant, childVariant);
    }
  }
  group.commonVariant = commonVariant;
  return commonVariant;
};

// Create a new group.
export const newGroup = (key: GroupKey, failureTime: string): FailureGroup => {
  return {
    id: key.value || nanoid(),
    key: key,
    criticalFailuresExonerated: 0,
    failures: 0,
    invocationFailures: 0,
    presubmitRejects: 0,
    children: [],
    isExpanded: false,
    latestFailureTime: failureTime,
    level: 0,
  };
};

// Find a group by key in the given list of groups, create a new one and insert it if it is not found.
// failureTime is only used when creating a new group.
export const getOrCreateGroup = (
  groups: FailureGroup[],
  key: GroupKey,
  failureTime: string,
): FailureGroup => {
  let group = groups.filter((g) => keyEqual(g.key, key))?.[0];
  if (group) {
    return group;
  }
  group = newGroup(key, failureTime);
  groups.push(group);
  return group;
};

// Returns the distinct values returned by featureExtractor for all children of the group.
// If featureExtractor returns undefined, the failure will be ignored.
// The distinct values for each group in the tree are also reported to `visitor` as the tree is traversed.
// A typical `visitor` function will store the count of distinct values in a property of the group.
export const treeDistinctValues = (
  group: FailureGroup,
  featureExtractor: FeatureExtractor,
  visitor: (group: FailureGroup, distinctValues: Set<string>) => void,
): Set<string> => {
  const values: Set<string> = new Set();
  if (group.failure) {
    for (const value of featureExtractor(group.failure)) {
      values.add(value);
    }
  } else {
    for (const child of group.children) {
      for (const value of treeDistinctValues(
        child,
        featureExtractor,
        visitor,
      )) {
        values.add(value);
      }
    }
  }
  visitor(group, values);
  return values;
};

// A FeatureExtractor returns a string representing some feature of a ClusterFailure.
// Returns undefined if there is no such feature for this failure.
export type FeatureExtractor = (failure: DistinctClusterFailure) => Set<string>;

// failureIdExtractor returns an extractor that returns a unique failure id for each failure.
// As failures don't actually have ids, it just returns an incrementing integer.
export const failureIdsExtractor = (): FeatureExtractor => {
  let unique = 0;
  return (f) => {
    const values: Set<string> = new Set();
    for (let i = 0; i < f.count; i++) {
      unique += 1;
      values.add('' + unique);
    }
    return values;
  };
};

// criticalFailuresExoneratedIdsExtractor returns an extractor that returns
// a unique failure id for each failure of a critical test that is exonerated.
// As failures don't actually have ids, it just returns an incrementing integer.
export const criticalFailuresExoneratedIdsExtractor = (): FeatureExtractor => {
  let unique = 0;
  return (f) => {
    const values: Set<string> = new Set();
    if (!f.isBuildCritical) {
      return values;
    }
    let exoneratedByCQ = false;
    if (f.exonerations !== null) {
      for (let i = 0; i < f.exonerations.length; i++) {
        // Do not count the exoneration reason NOT_CRITICAL
        // (as it implies the test is not critical), or the
        // exoneration reason UNEXPECTED_PASS as the test is considered
        // passing.
        // TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
        if (
          f.exonerations[i].reason === ExonerationReason.OCCURS_ON_OTHER_CLS
        ) {
          exoneratedByCQ = true;
        }
      }
    }
    if (!exoneratedByCQ) {
      return values;
    }

    for (let i = 0; i < f.count; i++) {
      unique += 1;
      values.add('' + unique);
    }
    return values;
  };
};

// Returns whether the failure was exonerated for a reason other than it occurred
// on other CLs or on mainline.
const isExoneratedByNonLUCIAnalysis = (
  exonerations: readonly DistinctClusterFailure_Exoneration[],
): boolean => {
  let hasOtherExoneration = false;
  for (let i = 0; i < exonerations.length; i++) {
    // TODO(b/250541091): Temporarily exclude OCCURS_ON_MAINLINE.
    if (exonerations[i].reason !== ExonerationReason.OCCURS_ON_OTHER_CLS) {
      hasOtherExoneration = true;
    }
  }
  return hasOtherExoneration;
};

// Returns an extractor that returns the id of the ingested invocation that was rejected by this failure, if any.
// The impact filter is taken into account in determining if the invocation was rejected by this failure.
export const rejectedIngestedInvocationIdsExtractor = (
  impactFilter: ImpactFilter,
): FeatureExtractor => {
  return (failure) => {
    const values: Set<string> = new Set();
    // If neither LUCI Analysis nor all exoneration is ignored, we want
    // actual impact.
    // This requires exclusion of all exonerated test results, as well as
    // test results from builds which passed (which implies the test results
    // could not have caused the presubmit run to fail).
    if (
      ((failure.exonerations !== undefined &&
        failure.exonerations.length > 0) ||
        failure.buildStatus !== BuildStatus.BUILD_STATUS_FAILURE) &&
      !(
        impactFilter.ignoreLUCIAnalysisExoneration ||
        impactFilter.ignoreAllExoneration
      )
    ) {
      return values;
    }
    // If not all exoneration is ignored, it means we want actual or without
    // LUCI Analysis impact.
    // All test results exonerated (except those exonerated by LUCI Analysis)
    // should be ignored.
    if (
      isExoneratedByNonLUCIAnalysis(failure.exonerations) &&
      !impactFilter.ignoreAllExoneration
    ) {
      return values;
    }
    if (
      !failure.isIngestedInvocationBlocked &&
      !impactFilter.ignoreIngestedInvocationBlocked
    ) {
      return values;
    }
    if (failure.ingestedInvocationId) {
      values.add(failure.ingestedInvocationId);
    }
    return values;
  };
};

// Returns an extractor that returns the identity of the CL that was rejected by this failure, if any.
// The impact filter is taken into account in determining if the CL was rejected by this failure.
export const rejectedPresubmitRunIdsExtractor = (
  impactFilter: ImpactFilter,
): FeatureExtractor => {
  return (failure) => {
    const values: Set<string> = new Set();
    // If neither LUCI Analysis nor all exoneration is ignored, we want
    // actual impact.
    // This requires exclusion of all exonerated test results, as well as
    // test results from builds which passed (which implies the test results
    // could not have caused the presubmit run to fail).
    if (
      ((failure.exonerations !== undefined &&
        failure.exonerations.length > 0) ||
        failure.buildStatus !== BuildStatus.BUILD_STATUS_FAILURE) &&
      !(
        impactFilter.ignoreLUCIAnalysisExoneration ||
        impactFilter.ignoreAllExoneration
      )
    ) {
      return values;
    }
    // If not all exoneration is ignored, it means we want actual or without
    // LUCI Analysis impact.
    // All test results exonerated (except those exonerated by LUCI Analysis)
    // should be ignored.
    if (
      isExoneratedByNonLUCIAnalysis(failure.exonerations) &&
      !impactFilter.ignoreAllExoneration
    ) {
      return values;
    }
    if (
      !failure.isIngestedInvocationBlocked &&
      !impactFilter.ignoreIngestedInvocationBlocked
    ) {
      return values;
    }
    if (
      failure.changelists !== undefined &&
      failure.changelists.length > 0 &&
      failure.presubmitRun !== undefined &&
      failure.presubmitRun.owner === 'user' &&
      failure.isBuildCritical &&
      failure.presubmitRun.mode === PresubmitRunMode.FULL_RUN
    ) {
      values.add(
        failure.changelists[0].host + '/' + failure.changelists[0].change,
      );
    }
    return values;
  };
};

// Sorts child failure groups at each node of the tree by the given metric.
export const sortFailureGroups = (
  groups: FailureGroup[],
  metric: MetricName,
  ascending: boolean,
): FailureGroup[] => {
  const cloneGroups = [...groups];
  const getMetric = (group: FailureGroup): number => {
    switch (metric) {
      case 'criticalFailuresExonerated':
        return group.criticalFailuresExonerated;
      case 'failures':
        return group.failures;
      case 'invocationFailures':
        return group.invocationFailures;
      case 'presubmitRejects':
        return group.presubmitRejects;
      case 'latestFailureTime':
        return DateTime.fromISO(group.latestFailureTime).toMillis();
      default:
        throw new Error('unknown metric: ' + metric);
    }
  };
  cloneGroups.sort((a, b) =>
    ascending ? getMetric(a) - getMetric(b) : getMetric(b) - getMetric(a),
  );
  for (const group of cloneGroups) {
    if (group.children.length > 0) {
      group.children = sortFailureGroups(group.children, metric, ascending);
    }
  }
  return cloneGroups;
};

/**
 * Groups failures by the variant groups selected.
 *
 * @param {DistinctClusterFailure} failures The list of failures to group.
 * @param {VariantGroup} variantGroups The list of variant groups to use for grouping.
 * @return {FailureGroup[]} The list of failures grouped by the variants.
 */
export const groupAndCountFailures = (
  failures: DistinctClusterFailure[],
  variantGroups: VariantGroup[],
): FailureGroup[] => {
  if (failures) {
    const groups = groupFailures(failures, (failure) => {
      const variantValues = variantGroups.map((v) => {
        const key: GroupKey = {
          type: 'variant',
          key: v.key,
          value: failure.variant?.def[v.key] || '',
        };
        return key;
      });
      return [...variantValues, { type: 'test', value: failure.testId || '' }];
    });
    return groups;
  }
  return [];
};

export const countFailures = (
  groups: FailureGroup[],
  impactFilter: ImpactFilter,
): FailureGroup[] => {
  const groupsClone = [...groups];
  groupsClone.forEach((group) => {
    treeDistinctValues(
      group,
      failureIdsExtractor(),
      (g, values) => (g.failures = values.size),
    );
    treeDistinctValues(
      group,
      criticalFailuresExoneratedIdsExtractor(),
      (g, values) => (g.criticalFailuresExonerated = values.size),
    );
    treeDistinctValues(
      group,
      rejectedIngestedInvocationIdsExtractor(impactFilter),
      (g, values) => (g.invocationFailures = values.size),
    );
    treeDistinctValues(
      group,
      rejectedPresubmitRunIdsExtractor(impactFilter),
      (g, values) => (g.presubmitRejects = values.size),
    );
  });
  return groupsClone;
};

// ImpactFilter represents what kind of impact should be counted or ignored in
// calculating impact for failures.
export interface ImpactFilter {
  id: string;
  name: string;
  ignoreLUCIAnalysisExoneration: boolean;
  ignoreAllExoneration: boolean;
  ignoreIngestedInvocationBlocked: boolean;
}
export const ImpactFilters: ImpactFilter[] = [
  {
    id: '',
    name: 'None (Actual Impact)',
    ignoreLUCIAnalysisExoneration: false,
    ignoreAllExoneration: false,
    ignoreIngestedInvocationBlocked: false,
  },
  {
    id: 'without-luci-exoneration',
    name: 'Without LUCI Analysis Exoneration',
    ignoreLUCIAnalysisExoneration: true,
    ignoreAllExoneration: false,
    ignoreIngestedInvocationBlocked: false,
  },
  {
    id: 'without-exoneration',
    name: 'Without All Exoneration',
    ignoreLUCIAnalysisExoneration: true,
    ignoreAllExoneration: true,
    ignoreIngestedInvocationBlocked: false,
  },
  {
    id: 'without-retries',
    name: 'Without Any Retries',
    ignoreLUCIAnalysisExoneration: true,
    ignoreAllExoneration: true,
    ignoreIngestedInvocationBlocked: true,
  },
];

export const defaultImpactFilter: ImpactFilter = ImpactFilters[0];

// Metrics that can be used for sorting FailureGroups.
// Each value is a property of FailureGroup.
export type MetricName =
  | 'presubmitRejects'
  | 'invocationFailures'
  | 'criticalFailuresExonerated'
  | 'failures'
  | 'latestFailureTime';

export type GroupType = 'test' | 'variant' | 'leaf';

export interface GroupKey {
  // The type of group.
  // This could be a group for a test, for a variant value,
  // or a leaf (for individual failures).
  type: GroupType;

  // For variant-based grouping keys, the name of the variant.
  // Unspecified otherwise.
  key?: string;

  // The name of the group. E.g. the name of the test or the variant value.
  // May be empty for leaf nodes.
  value: string;
}

export const keyEqual = (a: GroupKey, b: GroupKey) => {
  return a.value === b.value && a.key === b.key && a.type === b.type;
};

// FailureGroups are nodes in the failure tree hierarchy.
export interface FailureGroup {
  id: string;
  key: GroupKey;

  // Impact metrics for the group.
  criticalFailuresExonerated: number;
  failures: number;
  invocationFailures: number;
  presubmitRejects: number;
  latestFailureTime: string;

  level: number;

  // Child groups and expansion status. Only set for
  // non-leaf groups.
  children: FailureGroup[];
  isExpanded: boolean;

  // The failure. Only set for leaf groups.
  failure?: DistinctClusterFailure;

  // Variant key-value pairs that are common across all child failures.
  commonVariant?: Variant;
}

// VariantGroup represents variant key that appear on at least one failure.
export interface VariantGroup {
  key: string;
  values: string[];
}
