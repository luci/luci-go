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
import {
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { DrawerTreeNode } from '../components/test_navigation_drawer/types';

import {
  getTestDisplayName,
  normalizeDrawerFailureReason,
} from './test_variant_utils';

export interface HierarchyBuildResult {
  tree: DrawerTreeNode[];
  hasStructuredData: boolean; // True if hierarchy was built, false for flat list
  idsToExpand: string[]; // IDs of nodes to expand for the current test
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

const MAX_LABEL_LENGTH = 100; // Max length for a label segment before forced split

// Definition for generateVariantNode
function generateVariantNode(
  tv: TestVariant,
  variantResults: readonly TestResult[],
  failedCount: number,
  level: number,
): DrawerTreeNode {
  const variantString = Object.entries(tv.variant?.def || {})
    .map(([key, val]) => `${key}:${val}`)
    .join(', ');
  const primaryResultStatusV2 =
    failedCount > 0
      ? TestResult_Status.FAILED
      : variantResults.find((r) => r.statusV2 === TestResult_Status.PASSED)
          ?.statusV2 || TestResult_Status.STATUS_UNSPECIFIED;

  let tagText: string | undefined;
  let tagColorSemanticType: SemanticStatusType = getSemanticStatusFromResultV2(
    primaryResultStatusV2,
  );

  if (failedCount > 0) {
    tagText = `${failedCount} failed`;
    // tagColorSemanticType is already 'error' from getSemanticStatusFromResultV2
  } else if (variantResults.length === 0) {
    tagText = 'no results';
    tagColorSemanticType = 'unknown';
  }
  // Add overall TestVariant.status tag if informative and not already covered by a failure tag
  if (tv.status && tv.status !== TestVariantStatus.EXPECTED && !tagText) {
    tagColorSemanticType = getSemanticStatusFromTestVariant(tv.status);
    tagText = TestVariantStatus[tv.status]?.toLowerCase().replace(/_/g, ' ');
  }

  return {
    id: `${tv.testId}-${tv.variantHash}`,
    label: variantString || tv.variantHash?.substring(0, 12) || 'variant',
    level: level,
    isLeaf: true,
    testId: tv.testId,
    variantHash: tv.variantHash!,
    status: primaryResultStatusV2,
    failedTests: failedCount,
    totalTests: variantResults.length,
    isClickable: true,
    tag: tagText,
    tagColor: tagColorSemanticType,
  };
}

// TODO: This function does not work well for the moment, as we only have flat ids and
// the flat id algorithm splits into far too many levels.  Fix in later CL.
export function buildHierarchyTree(
  testVariants: readonly TestVariant[],
  currentTestId?: string,
  currentVariantHash?: string,
): HierarchyBuildResult {
  const defaultResult: HierarchyBuildResult = {
    tree: [],
    hasStructuredData: false,
    idsToExpand: [],
  };
  if (!testVariants || testVariants.length === 0) {
    return defaultResult;
  }

  // TODO: Implement robust logic to determine if actual structured data exists
  // (e.g., from TestResult.testIdStructured) to decide whether to use this hierarchical approach.
  // If not, fall back to the flat list logic.
  const attemptHierarchicalStructure = true; // Assume true for now

  const rootNodes: DrawerTreeNode[] = [];
  const nodeMap = new Map<string, DrawerTreeNode>(); // Tracks folder nodes by their full path ID
  const idsToExpandForCurrent: string[] = [];
  let rootNodeContainingCurrentTest: DrawerTreeNode | null = null;

  if (!attemptHierarchicalStructure) {
    // FLAT LIST LOGIC
    testVariants.forEach((tv) => {
      const variantResults =
        tv.results
          ?.map((rLink) => rLink.result)
          .filter((r): r is TestResult => !!r) || [];
      const failedCount = variantResults.filter(
        (r) => r.statusV2 === TestResult_Status.FAILED,
      ).length;
      const hasIndividualUnexpectedNonPass = variantResults.some(
        (r) => !r.expected && r.statusV2 !== TestResult_Status.PASSED,
      );
      const overallVariantSemanticStatus = getSemanticStatusFromTestVariant(
        tv.status,
      );
      const isOverallVariantProblematic =
        overallVariantSemanticStatus === 'error' ||
        overallVariantSemanticStatus === 'unexpectedly_skipped' ||
        overallVariantSemanticStatus === 'flaky';

      if (
        failedCount > 0 ||
        hasIndividualUnexpectedNonPass ||
        isOverallVariantProblematic
      ) {
        const displayName = getTestDisplayName(tv);
        const variantString = Object.entries(tv.variant?.def || {})
          .map(([k, v]) => `${k}:${v}`)
          .join(', ');
        const label = variantString
          ? `${displayName} (${variantString})`
          : displayName;
        let primaryDisplayStatusV2: TestResult_Status =
          TestResult_Status.STATUS_UNSPECIFIED;
        let tagText: string | undefined;
        let tagColorSemanticType: SemanticStatusType = 'neutral';

        if (failedCount > 0) {
          primaryDisplayStatusV2 = TestResult_Status.FAILED;
          tagText = `${failedCount} failed`;
          tagColorSemanticType = 'error';
        } else if (hasIndividualUnexpectedNonPass) {
          primaryDisplayStatusV2 =
            variantResults.find(
              (r) => !r.expected && r.statusV2 !== TestResult_Status.PASSED,
            )?.statusV2 || TestResult_Status.STATUS_UNSPECIFIED;
          tagText = 'unexpected result';
          tagColorSemanticType = 'warning';
        } else if (isOverallVariantProblematic) {
          tagText = TestVariantStatus[tv.status]
            ?.toLowerCase()
            .replace(/_/g, ' ');
          tagColorSemanticType = overallVariantSemanticStatus;
          if (tv.status === TestVariantStatus.UNEXPECTEDLY_SKIPPED)
            primaryDisplayStatusV2 = TestResult_Status.SKIPPED;
          else if (variantResults.length > 0)
            primaryDisplayStatusV2 =
              variantResults.find(
                (r) => r.statusV2 === TestResult_Status.PASSED,
              )?.statusV2 ||
              variantResults[0]?.statusV2 ||
              TestResult_Status.STATUS_UNSPECIFIED;
        } else if (
          variantResults.some((r) => r.statusV2 === TestResult_Status.PASSED)
        )
          primaryDisplayStatusV2 = TestResult_Status.PASSED;

        const nodeToAdd: DrawerTreeNode = {
          id: `${tv.testId}-${tv.variantHash}`,
          label: label,
          level: 0,
          isLeaf: true,
          testId: tv.testId,
          variantHash: tv.variantHash!,
          status: primaryDisplayStatusV2,
          failedTests: failedCount,
          totalTests: variantResults.length,
          isClickable: true,
          tag: tagText,
          tagColor: tagColorSemanticType,
        };
        if (
          tv.testId === currentTestId &&
          tv.variantHash === currentVariantHash
        ) {
          rootNodes.unshift(nodeToAdd);
          // For flat list, only the item itself might be "expanded" (selected)
          // No hierarchical IDs to expand beyond the item itself if it were a folder.
        } else {
          rootNodes.push(nodeToAdd);
        }
      }
    });
    return { tree: rootNodes, hasStructuredData: false, idsToExpand: [] };
  }

  // HIERARCHICAL LOGIC
  testVariants.forEach((tv) => {
    const displayName = getTestDisplayName(tv);
    // Split by any sequence of one or more non-alphanumeric characters.
    // Filter out empty strings that can result from consecutive delimiters.
    const originalPathSegments = displayName
      .split(/[^a-zA-Z0-9]+/)
      .filter(Boolean);

    const variantResults: readonly TestResult[] =
      tv.results
        ?.map((rLink) => rLink.result)
        .filter((r): r is TestResult => Boolean(r)) || [];
    const failedCount = variantResults.filter(
      (r) => r.statusV2 === TestResult_Status.FAILED,
    ).length;

    let currentParentNodeList = rootNodes;
    let accumulatedPathId = ''; // Used for nodeMap keys

    for (let i = 0; i < originalPathSegments.length; i++) {
      const segment = originalPathSegments[i];
      const isLastOriginalSegment = i === originalPathSegments.length - 1;

      // Handle length-based splitting for the current segment
      const subSegments: string[] = [];
      let tempSubSegment = segment;
      while (tempSubSegment.length > MAX_LABEL_LENGTH) {
        // For now, simple hard split. A real solution might try to find a sub-delimiter.
        subSegments.push(tempSubSegment.substring(0, MAX_LABEL_LENGTH));
        tempSubSegment = tempSubSegment.substring(MAX_LABEL_LENGTH);
      }
      subSegments.push(tempSubSegment); // Add the remainder

      for (let j = 0; j < subSegments.length; j++) {
        const currentSubSegmentLabel = subSegments[j];
        // Path ID must be unique for each node in the tree structure
        const newAccumulatedPathId = `${accumulatedPathId}${accumulatedPathId ? '___' : ''}${currentSubSegmentLabel}`;

        let node = nodeMap.get(newAccumulatedPathId);
        if (!node) {
          node = {
            id: `node-${newAccumulatedPathId}`, // Ensure ID is unique enough
            label: currentSubSegmentLabel,
            level: (nodeMap.get(accumulatedPathId)?.level ?? -1) + 1, // Level based on parent
            children: [],
            totalTests: 0,
            failedTests: 0,
            isClickable: false,
            isLeaf: false, // Assume folder, will be true only for variants or empty end folders
          };
          currentParentNodeList.push(node);
          nodeMap.set(newAccumulatedPathId, node);
        }

        node.totalTests = (node.totalTests || 0) + variantResults.length;
        node.failedTests = (node.failedTests || 0) + failedCount;

        // If this node corresponds to the currently viewed test variant's path, add to expand list
        if (
          tv.testId === currentTestId &&
          tv.variantHash === currentVariantHash
        ) {
          if (!idsToExpandForCurrent.includes(node.id)) {
            idsToExpandForCurrent.push(node.id);
          }
          if (
            currentParentNodeList === rootNodes &&
            !rootNodeContainingCurrentTest
          ) {
            rootNodeContainingCurrentTest = node;
          }
        }

        const isLastEffectiveSegment =
          isLastOriginalSegment && j === subSegments.length - 1;

        if (isLastEffectiveSegment) {
          // This is the deepest folder node for this test variant's path. Attach the variant.
          node.isLeaf = false; // It's a parent of variant(s)
          const variantNode = generateVariantNode(
            tv,
            variantResults,
            failedCount,
            node.level + 1,
          );
          node.children!.push(variantNode);
        } else {
          // This is an intermediate folder node. Prepare for the next segment.
          currentParentNodeList = node.children!;
          accumulatedPathId = newAccumulatedPathId; // The current node becomes the parent for the next segment
        }
      } // End of subSegments loop
      // After processing all sub-segments of an original segment,
      // if it wasn't the last original segment, currentParentNodeList is set for the next original segment.
      // If it was the last, the variant was attached.
      if (!isLastOriginalSegment) {
        // accumulatedPathId is already updated for the next segment's parent
      }
    } // End of originalPathSegments loop
  }); // End of testVariants.forEach

  // Reorder rootNodes to put the current item's branch at the top
  if (rootNodeContainingCurrentTest) {
    const idx = rootNodes.indexOf(rootNodeContainingCurrentTest);
    if (idx > 0) {
      rootNodes.splice(idx, 1);
      rootNodes.unshift(rootNodeContainingCurrentTest);
    }
  }
  // Ensure uniqueness and correct order (root to leaf) for expansion
  // The current logic adds to idsToExpandForCurrent as it traverses down, so it's already in order.
  return {
    tree: rootNodes,
    hasStructuredData: true,
    idsToExpand: Array.from(new Set(idsToExpandForCurrent)),
  };
}

export function buildFailureReasonTree(
  testVariants: readonly TestVariant[],
): DrawerTreeNode[] {
  if (!testVariants) return [];
  const reasons = new Map<
    string,
    {
      count: number;
      testVariantRefs: { testId: string; variantHash: string }[];
    }
  >();
  testVariants.forEach((tv) => {
    tv.results?.forEach((rLink) => {
      if (
        rLink.result?.statusV2 === TestResult_Status.FAILED &&
        rLink.result.failureReason?.primaryErrorMessage
      ) {
        const reasonKey = normalizeDrawerFailureReason(
          rLink.result.failureReason.primaryErrorMessage,
        );
        if (!reasons.has(reasonKey)) {
          reasons.set(reasonKey, { count: 0, testVariantRefs: [] });
        }
        const reasonEntry = reasons.get(reasonKey)!;
        reasonEntry.count++;
        if (tv.variantHash) {
          reasonEntry.testVariantRefs.push({
            testId: tv.testId,
            variantHash: tv.variantHash,
          });
        }
      }
    });
  });

  return Array.from(reasons.entries()).map(([reason, data], index) => ({
    id: `failure-${index}-${encodeURIComponent(reason)}`,
    label: reason,
    level: 0,
    isLeaf: true,
    isClickable: false,
    totalTests: data.count, // Using totalTests to store the count of this failure reason
    tag: `${data.count} failed`,
    tagColor: 'error', // SemanticStatusType
  }));
}
