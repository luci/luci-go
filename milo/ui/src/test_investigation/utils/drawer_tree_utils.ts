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

import { SemanticStatusType } from '@/common/styles/status_styles';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  TestNavigationTreeGroup,
  TestNavigationTreeNode,
} from '../components/test_navigation_drawer/types';

import { normalizeDrawerFailureReason } from './test_variant_utils';

export interface HierarchyBuildResult {
  tree: TestNavigationTreeNode[];
  // TODO: IDs of nodes leading to the currently selected test variant.
  idsToExpand: string[];
}

// Helper to map TestResult_Status (from a result's statusV2 field) to a SemanticStatusType.
export function getSemanticStatusFromResultV2(
  statusV2?: TestResult_Status,
): SemanticStatusType {
  if (statusV2 === undefined) return 'unknown';
  switch (statusV2) {
    case TestResult_Status.PASSED:
      return 'success';
    case TestResult_Status.FAILED:
      return 'error';
    case TestResult_Status.SKIPPED:
      return 'skipped';
    case TestResult_Status.EXECUTION_ERRORED:
      return 'infra_failure';
    case TestResult_Status.PRECLUDED:
      return 'skipped'; // Or a specific 'precluded'
    case TestResult_Status.STATUS_UNSPECIFIED:
    default:
      return 'unknown';
  }
}

// Helper to map TestVariantStatus (overall status of a test variant) to a SemanticStatusType.
export function getSemanticStatusFromTestVariant(
  status?: TestVariantStatus,
): SemanticStatusType {
  if (status === undefined) return 'unknown';
  switch (status) {
    case TestVariantStatus.EXPECTED:
      return 'expected';
    case TestVariantStatus.UNEXPECTED:
      return 'error'; // Maps to "failing"
    case TestVariantStatus.UNEXPECTEDLY_SKIPPED:
      return 'unexpectedly_skipped';
    case TestVariantStatus.FLAKY:
      return 'flaky';
    case TestVariantStatus.EXONERATED:
      return 'exonerated';
    default:
      return 'unknown';
  }
}

// Helper to map TestResult_Status (from a result's statusV2 field) to a SemanticStatusType.
export function getSemanticStatusFromVerdict(
  statusV2?: TestVerdict_Status,
): SemanticStatusType {
  if (statusV2 === undefined) return 'unknown';
  switch (statusV2) {
    case TestVerdict_Status.FLAKY:
      return 'warning';
    case TestVerdict_Status.PASSED:
      return 'success';
    case TestVerdict_Status.FAILED:
      return 'error';
    case TestVerdict_Status.SKIPPED:
      return 'skipped';
    case TestVerdict_Status.EXECUTION_ERRORED:
      return 'infra_failure';
    case TestVerdict_Status.PRECLUDED:
      return 'skipped';
    default:
      return 'unknown';
  }
}

export function buildHierarchyTree(
  testVariants: readonly TestVariant[],
): HierarchyBuildResult {
  const result: HierarchyBuildResult = {
    tree: [],
    idsToExpand: [],
  };
  if (!testVariants || testVariants.length === 0) {
    return result;
  }

  // Algorithm:
  // 1. Put any test variants with structured ids into a tree based on the structured ids
  const structuredVariants = testVariants.filter(
    (tv) =>
      !!tv.testIdStructured && tv.testIdStructured.moduleScheme !== 'legacy',
  );
  result.tree = buildStructuredTree(
    StructuredTreeLevel.Module,
    structuredVariants,
  );
  // 2. For the remaining test variants, build up a hierarchy based on longest common prefixes and common separator characters.
  const flatVariants = testVariants.filter(
    (tv) => !tv.testIdStructured || tv.testIdStructured.moduleName === 'legacy',
  );
  result.tree = [
    ...result.tree,
    ...compressSingleChildNodes(buildFlatTree(flatVariants)),
  ];

  return result;
}

export enum StructuredTreeLevel {
  Module,
  Variant,
  Coarse,
  Fine,
  Case,
}

export function structuredTreeLevelData(
  level: StructuredTreeLevel,
  tv: TestVariant,
): string | undefined {
  switch (level) {
    case StructuredTreeLevel.Module:
      return tv.testIdStructured?.moduleName;
    case StructuredTreeLevel.Variant:
      return tv.testIdStructured?.moduleVariantHash;
    case StructuredTreeLevel.Coarse:
      return tv.testIdStructured?.coarseName;
    case StructuredTreeLevel.Fine:
      return tv.testIdStructured?.fineName;
    case StructuredTreeLevel.Case:
      return tv.testIdStructured?.caseName;
    default:
      return undefined;
  }
}

export function buildStructuredTree(
  level: StructuredTreeLevel,
  variants: TestVariant[],
): TestNavigationTreeNode[] {
  if (!variants || variants.length === 0 || level > StructuredTreeLevel.Case) {
    return [];
  }
  const groups = new Map<string, TestVariant[]>();
  variants.forEach((tv) => {
    const data = structuredTreeLevelData(level, tv) ?? '';
    if (!groups.has(data)) {
      groups.set(data, []);
    }
    groups.get(data)!.push(tv);
  });

  const nodes: TestNavigationTreeNode[] = [];
  if (level < StructuredTreeLevel.Case) {
    groups.forEach((groupVariants, data) => {
      const children = buildStructuredTree(level + 1, groupVariants);
      if (children.length > 0) {
        if (data === '') {
          // This level was skipped for this group of variants.
          // Instead of creating a node with an empty label,
          // just add its children to the current list of nodes.
          nodes.push(...children);
        } else {
          nodes.push({
            id: `${level}-${data}`,
            label: data,
            level: level,
            children: children,
            failedTests: children.reduce(
              (sum, child) => sum + child.failedTests,
              0,
            ),
            passedTests: children.reduce(
              (sum, child) => sum + child.passedTests,
              0,
            ),
            flakyTests: children.reduce(
              (sum, child) => sum + child.flakyTests,
              0,
            ),
            skippedTests: children.reduce(
              (sum, child) => sum + child.skippedTests,
              0,
            ),
            errorTests: children.reduce(
              (sum, child) => sum + child.errorTests,
              0,
            ),
            precludedTests: children.reduce(
              (sum, child) => sum + child.precludedTests,
              0,
            ),
            unknownTests: children.reduce(
              (sum, child) => sum + child.unknownTests,
              0,
            ),
            totalTests: children.reduce(
              (sum, child) => sum + child.totalTests,
              0,
            ),
            isStructured: true,
          });
        }
      }
    });
  } else {
    groups.forEach((variants, data) => {
      nodes.push({
        id: `${level}-${data}`,
        label: data,
        level: level,
        totalTests: 1,
        failedTests: variants[0].statusV2 === TestVerdict_Status.FAILED ? 1 : 0,
        passedTests: variants[0].statusV2 === TestVerdict_Status.PASSED ? 1 : 0,
        flakyTests: variants[0].statusV2 === TestVerdict_Status.FLAKY ? 1 : 0,
        skippedTests:
          variants[0].statusV2 === TestVerdict_Status.SKIPPED ? 1 : 0,
        errorTests:
          variants[0].statusV2 === TestVerdict_Status.EXECUTION_ERRORED ? 1 : 0,
        precludedTests:
          variants[0].statusV2 === TestVerdict_Status.PRECLUDED ? 1 : 0,
        unknownTests: variants[0].statusV2 === undefined ? 1 : 0,
        testVariant: variants[0],
        isStructured: true,
      });
    });
  }

  return nodes;
}
export interface FlatTreeEntry {
  path: string[];
  value: TestVariant;
}
export function buildFlatTree(
  variants: TestVariant[],
): TestNavigationTreeNode[] {
  const entries = variants.map((tv): FlatTreeEntry => {
    return {
      path: pathSplit(tv.testId),
      value: tv,
    };
  });
  return buildFlatTreeFromEntries(0, entries);
}

export function buildFlatTreeFromEntries(
  level: number,
  entries: FlatTreeEntry[],
): TestNavigationTreeNode[] {
  if (!entries || entries.length === 0) {
    return [];
  }

  const groups = new Map<string, FlatTreeEntry[]>();
  const leaves: TestNavigationTreeNode[] = [];
  entries.forEach((entry) => {
    const component = entry.path[level] || '';
    if (entry.path.length - 1 === level) {
      leaves.push({
        id: entry.path.slice(0, level + 1).join(''),
        label: component,
        level: level,
        totalTests: 1,
        failedTests: entry.value.statusV2 === TestVerdict_Status.FAILED ? 1 : 0,
        passedTests: entry.value.statusV2 === TestVerdict_Status.PASSED ? 1 : 0,
        flakyTests: entry.value.statusV2 === TestVerdict_Status.FLAKY ? 1 : 0,
        skippedTests:
          entry.value.statusV2 === TestVerdict_Status.SKIPPED ? 1 : 0,
        errorTests:
          entry.value.statusV2 === TestVerdict_Status.EXECUTION_ERRORED ? 1 : 0,
        precludedTests:
          entry.value.statusV2 === TestVerdict_Status.PRECLUDED ? 1 : 0,
        unknownTests: entry.value.statusV2 === undefined ? 1 : 0,
        testVariant: entry.value,
        isStructured: false,
      });
    } else {
      if (!groups.has(component)) {
        groups.set(component, []);
      }
      groups.get(component)!.push(entry);
    }
  });

  const nodes: TestNavigationTreeNode[] = [];
  groups.forEach((groupEntries, component) => {
    const children = buildFlatTreeFromEntries(level + 1, groupEntries);
    if (children.length > 0) {
      nodes.push({
        id: groupEntries[0].path.slice(0, level + 1).join(''),
        label: component,
        level: level,
        totalTests: children.reduce((sum, child) => sum + child.totalTests, 0),
        failedTests: children.reduce(
          (sum, child) => sum + child.failedTests,
          0,
        ),
        passedTests: children.reduce(
          (sum, child) => sum + child.passedTests,
          0,
        ),
        flakyTests: children.reduce((sum, child) => sum + child.flakyTests, 0),
        skippedTests: children.reduce(
          (sum, child) => sum + child.skippedTests,
          0,
        ),
        errorTests: children.reduce((sum, child) => sum + child.errorTests, 0),
        precludedTests: children.reduce(
          (sum, child) => sum + child.precludedTests,
          0,
        ),
        unknownTests: children.reduce(
          (sum, child) => sum + child.unknownTests,
          0,
        ),
        isStructured: false,
        children: children,
      });
    }
  });
  return [...nodes, ...leaves];
}

/**
 * Split the id string into components separated by non alpha-numeric components
 * and preserving the split character at the end of each component string.
 */
export function pathSplit(id: string): string[] {
  const parts: string[] = [];
  let currentPart = '';
  for (let i = 0; i < id.length; i++) {
    const char = id[i];
    if (/[a-zA-Z0-9_-]/.test(char)) {
      currentPart += char;
    } else {
      if (currentPart) {
        parts.push(currentPart + char);
        currentPart = '';
      } else {
        // Handle consecutive separators or leading separators
        parts.push(char);
      }
    }
  }
  if (currentPart) {
    parts.push(currentPart);
  }
  return parts;
}

/**
 * Compress any nodes that only have a single child by combining the parent and child node
 * into a single node with a concatenated label, and the children of the child node.
 */
export function compressSingleChildNodes(
  nodes: TestNavigationTreeNode[],
): TestNavigationTreeNode[] {
  const compressed = nodes.map((parent) => {
    if (parent.children) {
      parent.children = compressSingleChildNodes(parent.children);
      if (parent.children.length === 1) {
        const child = parent.children[0];
        return {
          ...parent,
          id: `${parent.id}-${child.id}`,
          label: `${parent.label}${child.label}`,
          children: child.children,
          testVariant: child.testVariant,
        };
      }
    }
    return parent;
  });

  // Make sure any nodes that became leaves appear after all non-leaf nodes.
  const stillNodes = compressed.filter((n) => n.children);
  const leaves = compressed.filter((n) => !n.children);
  return [...stillNodes, ...leaves];
}

export function buildFailureReasonTree(
  testVariants: readonly TestVariant[],
): TestNavigationTreeGroup[] {
  if (!testVariants) return [];
  const reasons = new Map<string, TestVariant[]>();
  testVariants.forEach((tv) => {
    tv.results?.forEach((rLink) => {
      if (rLink.result?.statusV2 === TestResult_Status.FAILED) {
        const reasonKey = normalizeDrawerFailureReason(
          rLink.result.failureReason?.primaryErrorMessage || '',
        );
        if (!reasons.has(reasonKey)) {
          reasons.set(reasonKey, []);
        }
        const reasonVariants = reasons.get(reasonKey)!;
        if (reasonVariants.indexOf(tv) === -1) {
          reasonVariants.push(tv);
        }
      }
    });
  });

  return Array.from(reasons.entries()).map(
    ([reason, variants], index): TestNavigationTreeGroup => {
      const nodes = buildHierarchyTree(variants).tree;
      return {
        id: `failure-${index}-${encodeURIComponent(reason)}`,
        label: reason,
        nodes,
        totalTests: nodes.reduce((sum, child) => sum + child.totalTests, 0),
        failedTests: nodes.reduce((sum, child) => sum + child.failedTests, 0),
        passedTests: nodes.reduce((sum, child) => sum + child.passedTests, 0),
        flakyTests: nodes.reduce((sum, child) => sum + child.flakyTests, 0),
        skippedTests: nodes.reduce((sum, child) => sum + child.skippedTests, 0),
        errorTests: nodes.reduce((sum, child) => sum + child.errorTests, 0),
        precludedTests: nodes.reduce(
          (sum, child) => sum + child.precludedTests,
          0,
        ),
        unknownTests: nodes.reduce((sum, child) => sum + child.unknownTests, 0),
      };
    },
  );
}
