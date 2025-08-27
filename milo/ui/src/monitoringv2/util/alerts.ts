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

import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

export type GenericAlert = BuilderAlert | StepAlert | TestAlert;
export type AlertKind = 'builder' | 'step' | 'test';

export interface BuilderAlert {
  kind: 'builder';
  key: string;
  builderID: BuilderID;
  history: OneBuildHistory[];
  consecutiveFailures: number;
  consecutivePasses: number;
}

export interface StepAlert {
  kind: 'step';
  key: string;
  builderID: BuilderID;
  stepName: string;
  history: OneBuildHistory[];
  consecutiveFailures: number;
  consecutivePasses: number;
}

export interface TestAlert {
  kind: 'test';
  key: string;
  builderID: BuilderID;
  stepName: string;
  testName: string;
  testId: string;
  variantHash: string;
  history: OneTestHistory[];
  consecutiveFailures: number;
  consecutivePasses: number;
}

export interface OneBuildHistory {
  buildId: string;
  status: Status | undefined;
  startTime?: string;
  summaryMarkdown?: string;
}

export interface OneTestHistory {
  buildId: string;
  status: TestVariantStatus | undefined;
  startTime?: string;
  failureReason: string | undefined;
}

export interface StructuredAlert {
  alert: GenericAlert;
  children: StructuredAlert[];
  consecutiveFailures: number;
  consecutivePasses: number;
}

export interface AlertOrganzerOptions {
  showResolved: boolean;
  showFlaky: boolean;
  showChildrenHidden: boolean;
}

export type GroupedAlertKeys = { [groupId: string]: readonly string[] };

export class AlertOrganizer {
  private options: AlertOrganzerOptions;
  private groups: GroupedAlertKeys;
  private structuredAlerts: StructuredAlert[];

  constructor(
    alerts: GenericAlert[],
    groups: GroupedAlertKeys,
    options: AlertOrganzerOptions,
  ) {
    this.options = options;
    this.groups = groups;
    this.structuredAlerts = buildStructuredAlerts(alerts);
  }

  allAlerts(kind: AlertKind): StructuredAlert[] {
    const all = this.structuredAlerts.filter((a) => a.alert.kind === kind);
    return cloneShownAlerts(all, {}, this.options);
  }

  ungroupedAlerts(kind: AlertKind): StructuredAlert[] {
    const all = this.structuredAlerts.filter((a) => a.alert.kind === kind);
    const alertKeys = Object.fromEntries(
      Object.entries(this.groups)
        .flatMap(([_, keys]) => keys)
        .map((k) => [k, true]),
    );
    return cloneShownAlerts(all, alertKeys, this.options);
  }

  groupAlerts(groupId: string): StructuredAlert[] {
    const groupAlertKeys = this.groups[groupId] || [];
    const topLevel = this.structuredAlerts.filter((a) =>
      groupAlertKeys.includes(a.alert.key),
    );
    return cloneShownAlerts(topLevel, {}, this.options);
  }
}

const cloneShownAlerts = (
  alerts: StructuredAlert[],
  hiddenKeys: { [alertKey: string]: boolean },
  options: AlertOrganzerOptions,
): StructuredAlert[] => {
  const cloned: StructuredAlert[] = [];
  for (const alert of alerts) {
    if (hiddenKeys[alert.alert.key]) {
      continue;
    }
    if (alert.alert.consecutiveFailures === 0 && !options.showResolved) {
      continue;
    }
    if (alert.alert.consecutiveFailures === 1 && !options.showFlaky) {
      continue;
    }
    const clone = { ...alert };
    clone.children = cloneShownAlerts(alert.children, hiddenKeys, options);
    if (
      clone.children.length === 0 &&
      alert.children.length > 0 &&
      !options.showChildrenHidden
    ) {
      continue;
    }
    cloned.push(clone);
  }
  return cloned;
};

export const filterResolved = (alerts: GenericAlert[]): GenericAlert[] =>
  alerts.filter((a) => a.consecutiveFailures > 0);

export const buildStructuredAlerts = (
  alerts: GenericAlert[],
): StructuredAlert[] => {
  const builderAlerts = alerts.filter((a) => a.kind === 'builder');
  const stepAlerts = alerts.filter((a) => a.kind === 'step');
  const testAlerts = alerts.filter((a) => a.kind === 'test');

  return [
    ...organizeRelatedAlerts([builderAlerts, stepAlerts, testAlerts]),
    ...organizeRelatedAlerts([stepAlerts, builderAlerts, testAlerts]),
    ...organizeRelatedAlerts([testAlerts, stepAlerts, builderAlerts]),
  ];
};

/**
 *
 * organizeRelatedAlerts recursively organizes alerts into a tree structure based on their keys.
 * The function takes an array of alert groups as input, where each group represents a different
 * type of alert (builder, step, test). It iteratively organizes alerts from each group into a
 * tree structure, ensuring that parent-child relationships are correctly represented.
 *
 * @param alertGroups An array of alert groups, each containing alerts of a specific type.
 * @returns An array of StructuredAlert objects representing the organized alert tree.
 */
export const organizeRelatedAlerts = (
  alertGroups: GenericAlert[][],
): StructuredAlert[] => {
  if (alertGroups.length === 0) {
    // Should never happen.
    return [];
  }
  const alerts = alertGroups[0].map(makeStructuredAlert);
  for (let level = 1; level < alertGroups.length; level++) {
    alerts.forEach((alert) => {
      const childKeys = alert.children.map((a) => a.alert.key);
      alert.children = [
        ...alert.children,
        ...sortAlertsByFailurePattern(
          organizeRelatedAlerts([
            alertGroups[level].filter(
              (a) =>
                isAlertKeyRelated(a.key, alert.alert.key) &&
                !childKeys.find((k) => isAlertKeyRelated(k, a.key)),
            ),
            ...alertGroups.slice(level + 1),
          ]),
          alert.consecutiveFailures,
        ),
      ];
    });
  }
  return alerts;
};

export const isAlertKeyRelated = (a: string, b: string): boolean => {
  return a.startsWith(b) || b.startsWith(a);
};

export const makeStructuredAlert = (alert: GenericAlert): StructuredAlert => {
  return {
    alert,
    children: [],
    consecutiveFailures: alert.consecutiveFailures,
    consecutivePasses: alert.consecutivePasses,
  };
};

/**
 * sortAlertsByFailurePattern attempts to sort child alerts in the order most interesting to
 * a user looking at them.
 *
 * First present any alerts where the number of consecutive failures matches the parent alert
 * exactly.  These are most likely to be the cause of the parent failure.
 *
 * Next present any alerts where the number of consecustive failures is less than the parent,
 * in descending order of the consecutive failures.  If there are no exact matches these are
 * most likely to be the cause (with earlier failures having a different cause).
 *
 * Next present any alerts with a larger number of consecutive failures than the parent.  These
 * are unlikely to be the cause because the parent passed when these were failing.
 *
 * Last present any alerts that have zero consecutive failures, these are already resolved and
 * Are likely only of interest for verifying a fix.
 *
 * For any alerts that are equal under the above rules, break ties with a localeCompare of the alert keys.
 *
 * @param alerts The child alerts to sort
 * @param parentConsecutiveFailures The number of consecutive failures observed in the parent of these alerts.
 * @returns The child alerts sorted as described above.
 */
export const sortAlertsByFailurePattern = (
  alerts: StructuredAlert[],
  parentConsecutiveFailures: number,
): StructuredAlert[] => {
  return alerts.sort((a, b) => {
    if (a.consecutiveFailures === b.consecutiveFailures) {
      if (a.consecutiveFailures === 0) {
        return a.consecutivePasses === b.consecutivePasses
          ? a.alert.key.localeCompare(b.alert.key)
          : a.consecutivePasses - b.consecutivePasses;
      }
      return a.alert.key.localeCompare(b.alert.key);
    }
    if (a.consecutiveFailures === parentConsecutiveFailures) {
      return b.consecutiveFailures === parentConsecutiveFailures
        ? a.alert.key.localeCompare(b.alert.key)
        : -1;
    } else if (b.consecutiveFailures === parentConsecutiveFailures) {
      return 1;
    }

    if (a.consecutiveFailures < parentConsecutiveFailures) {
      return b.consecutiveFailures < parentConsecutiveFailures
        ? b.consecutiveFailures - a.consecutiveFailures
        : -1;
    } else if (b.consecutiveFailures < parentConsecutiveFailures) {
      return 1;
    }

    return a.consecutiveFailures - b.consecutiveFailures;
  });
};

export interface CategorizedAlerts {
  // Alerts not associated with a bug that have occurred in more than one consecutive build.
  consistentFailures: StructuredAlert[];
  // Alerts not associated with bugs that have only happened once.
  newFailures: StructuredAlert[];
  bugAlerts: { [bug: string]: StructuredAlert[] };
}

// filterAlerts returns the alerts that match the given filter string typed by the user.
// alerts can currently match in any part of the key (i.e. builder, step test id).
// In the future this can be extended to whatever else is useful to users.
export const filterAlerts = (
  alerts: StructuredAlert[],
  filter: string,
): StructuredAlert[] => {
  if (filter === '') {
    return alerts;
  }
  const re = new RegExp(filter, 'i');
  return alerts.filter((alert) => {
    if (re.test(alert.alert.key)) {
      return true;
    }
    return filterAlerts(alert.children, filter).length > 0;
  });
};

// Code for sorting structured alerts based on a field.
export type SortColumn = 'failure' | 'history';
export type SortDirection = 'asc' | 'desc';

export const sortAlerts = (
  alerts: StructuredAlert[],
  sortColumn: SortColumn | undefined,
  sortDirection: SortDirection,
): StructuredAlert[] => {
  if (!sortColumn) {
    return alerts;
  }
  return alerts.sort((a, b) => {
    const aValue = sortValue(a, sortColumn);
    const bValue = sortValue(b, sortColumn);
    if (sortDirection === 'desc') {
      return aValue < bValue ? 1 : aValue > bValue ? -1 : 0;
    } else {
      return aValue < bValue ? -1 : aValue > bValue ? 1 : 0;
    }
  });
};

const sortValue = (a: StructuredAlert, column: SortColumn): string | number => {
  switch (column) {
    case 'failure':
      return a.alert.key;
    case 'history':
      return a.alert.consecutiveFailures;
    default:
      throw new Error(`Unknown sort column: ${column}`);
  }
};
