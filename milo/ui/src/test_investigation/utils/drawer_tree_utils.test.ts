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

import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import {
  TestVariant,
  TestVariantStatus,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { TestNavigationTreeNode } from '../components/test_navigation_drawer/types';

import {
  buildFailureReasonTree,
  buildFlatTree,
  buildHierarchyTreeAndFindExpandedIds,
  buildStructuredTree,
  compressSingleChildNodes,
  getSemanticStatusFromResultV2,
  getSemanticStatusFromTestVariant,
  getSemanticStatusFromVerdict,
  pathSplit,
  StructuredTreeLevel,
} from './drawer_tree_utils';
import { normalizeDrawerFailureReason } from './test_variant_utils';

describe('DrawerTreeUtils', () => {
  describe('getSemanticStatusFromResultV2', () => {
    it.each([
      [TestResult_Status.PASSED, 'success'],
      [TestResult_Status.FAILED, 'error'],
      [TestResult_Status.SKIPPED, 'skipped'],
      [TestResult_Status.EXECUTION_ERRORED, 'infra_failure'],
      [TestResult_Status.PRECLUDED, 'skipped'],
      [TestResult_Status.STATUS_UNSPECIFIED, 'unknown'],
      [undefined, 'unknown'],
    ])('should map status %p to %p', (status, expected) => {
      expect(getSemanticStatusFromResultV2(status)).toBe(expected);
    });
  });

  describe('getSemanticStatusFromTestVariant', () => {
    it.each([
      [TestVariantStatus.EXPECTED, 'expected'],
      [TestVariantStatus.UNEXPECTED, 'error'],
      [TestVariantStatus.UNEXPECTEDLY_SKIPPED, 'unexpectedly_skipped'],
      [TestVariantStatus.FLAKY, 'flaky'],
      [TestVariantStatus.EXONERATED, 'exonerated'],
      [undefined, 'unknown'],
    ])('should map status %p to %p', (status, expected) => {
      expect(getSemanticStatusFromTestVariant(status)).toBe(expected);
    });
  });

  describe('getSemanticStatusFromVerdict', () => {
    it.each([
      [TestVerdict_Status.FLAKY, 'warning'],
      [TestVerdict_Status.PASSED, 'success'],
      [TestVerdict_Status.FAILED, 'error'],
      [TestVerdict_Status.SKIPPED, 'skipped'],
      [TestVerdict_Status.EXECUTION_ERRORED, 'infra_failure'],
      [TestVerdict_Status.PRECLUDED, 'skipped'],
      [undefined, 'unknown'],
    ])('should map status %p to %p', (status, expected) => {
      expect(getSemanticStatusFromVerdict(status)).toBe(expected);
    });
  });

  describe('buildStructuredTree', () => {
    it('should handle missing hierarchy levels gracefully', () => {
      const variants: TestVariant[] = [
        {
          testIdStructured: {
            moduleName: 'infra/luci/luci-go > //:go_tests',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'e3b0c44298fc1c14',
            fineName:
              'go.chromium.org/luci/resultdb/internal/services/resultdb',
            caseName: 'TestBatchGetTestVariants',
          },
          statusV2: TestVerdict_Status.FAILED,
        },
        {
          testIdStructured: {
            moduleName: 'infra/luci/luci-go > //:go_tests',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'e3b0c44298fc1c14',
            fineName:
              'go.chromium.org/luci/resultdb/internal/services/resultdb',
            caseName: 'TestBatchGetTestVariants/BatchGetTestVariants',
          },
          statusV2: TestVerdict_Status.FAILED,
        },
        {
          testIdStructured: {
            moduleName: 'infra/luci/luci-go > //:go_tests',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'e3b0c44298fc1c14',
            fineName:
              'go.chromium.org/luci/resultdb/internal/services/resultdb',
            caseName: 'TestValidListArtifactLinesRequest',
          },
          statusV2: TestVerdict_Status.EXECUTION_ERRORED,
        },
        {
          testIdStructured: {
            moduleName: 'infra/luci/luci-go > //:go_tests',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'e3b0c44298fc1c14',
            fineName:
              'go.chromium.org/luci/resultdb/internal/services/resultdb',
            caseName: '*fixture',
          },
          statusV2: TestVerdict_Status.PRECLUDED,
        },
      ] as TestVariant[];

      const tree = buildStructuredTree(StructuredTreeLevel.Module, variants);

      expect(tree).toHaveLength(1);
      const moduleNode = tree[0];
      expect(moduleNode.label).toBe('infra/luci/luci-go > //:go_tests');
      expect(moduleNode.totalTests).toBe(4);
      expect(moduleNode.failedTests).toBe(2);

      expect(moduleNode.children).toHaveLength(1);
      const variantNode = moduleNode.children![0];
      expect(variantNode.label).toBe('e3b0c44298fc1c14');
      expect(variantNode.totalTests).toBe(4);
      expect(variantNode.failedTests).toBe(2);

      // Coarse level is skipped, so the next level should be fine level.
      expect(variantNode.children).toHaveLength(1);
      const fineNode = variantNode.children![0];
      expect(fineNode.label).toBe(
        'go.chromium.org/luci/resultdb/internal/services/resultdb',
      );
      expect(fineNode.totalTests).toBe(4);
      expect(fineNode.failedTests).toBe(2);

      expect(fineNode.children).toHaveLength(4);
      const caseNodes = fineNode.children!.sort((a, b) =>
        a.label.localeCompare(b.label),
      );

      expect(caseNodes[0].label).toBe('*fixture');
      expect(caseNodes[0].totalTests).toBe(1);
      expect(caseNodes[0].failedTests).toBe(0);

      expect(caseNodes[1].label).toBe('TestBatchGetTestVariants');
      expect(caseNodes[1].totalTests).toBe(1);
      expect(caseNodes[1].failedTests).toBe(1);

      expect(caseNodes[2].label).toBe(
        'TestBatchGetTestVariants/BatchGetTestVariants',
      );
      expect(caseNodes[2].totalTests).toBe(1);
      expect(caseNodes[2].failedTests).toBe(1);

      expect(caseNodes[3].label).toBe('TestValidListArtifactLinesRequest');
      expect(caseNodes[3].totalTests).toBe(1);
      expect(caseNodes[3].failedTests).toBe(0);
    });

    it('should build a tree with all hierarchy levels', () => {
      const variants: TestVariant[] = [
        {
          testIdStructured: {
            moduleName: 'module1',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'hash1',
            coarseName: 'coarse1',
            fineName: 'fine1',
            caseName: 'case1',
          },
          statusV2: TestVerdict_Status.FAILED,
        },
        {
          testIdStructured: {
            moduleName: 'module1',
            moduleScheme: 'go',
            moduleVariant: {},
            moduleVariantHash: 'hash1',
            coarseName: 'coarse1',
            fineName: 'fine1',
            caseName: 'case2',
          },
          statusV2: TestVerdict_Status.PASSED,
        },
      ] as TestVariant[];

      const tree = buildStructuredTree(StructuredTreeLevel.Module, variants);

      expect(tree).toHaveLength(1); // module1
      const moduleNode = tree[0];
      expect(moduleNode.label).toBe('module1');
      expect(moduleNode.totalTests).toBe(2);
      expect(moduleNode.failedTests).toBe(1);

      expect(moduleNode.children).toHaveLength(1); // hash1
      const variantNode = moduleNode.children![0];
      expect(variantNode.label).toBe('hash1');

      expect(variantNode.children).toHaveLength(1); // coarse1
      const coarseNode = variantNode.children![0];
      expect(coarseNode.label).toBe('coarse1');

      expect(coarseNode.children).toHaveLength(1); // fine1
      const fineNode = coarseNode.children![0];
      expect(fineNode.label).toBe('fine1');

      expect(fineNode.children).toHaveLength(2); // case1, case2
      const case1 = fineNode.children![0];
      expect(case1.label).toBe('case1');
      expect(case1.failedTests).toBe(1);
      const case2 = fineNode.children![1];
      expect(case2.label).toBe('case2');
      expect(case2.failedTests).toBe(0);
    });
  });

  describe('pathSplit', () => {
    it('should split a simple path', () => {
      expect(pathSplit('a/b/c')).toEqual(['a/', 'b/', 'c']);
    });

    it('should ignore different separators', () => {
      expect(pathSplit('a.b::c>d')).toEqual(['a.b::c>d']);
    });

    it('should handle leading and trailing separators', () => {
      expect(pathSplit('/a/b/')).toEqual(['/', 'a/', 'b/']);
    });

    it('should handle consecutive separators', () => {
      expect(pathSplit('a//b')).toEqual(['a/', '/', 'b']);
    });

    it('should handle strings with no separators', () => {
      expect(pathSplit('abc')).toEqual(['abc']);
    });

    it('should handle special characters in parts', () => {
      expect(pathSplit('part-1_with_stuff/part-2')).toEqual([
        'part-1_with_stuff/',
        'part-2',
      ]);
    });
  });

  describe('compressSingleChildNodes', () => {
    it('should compress nodes with a single child', () => {
      const tree: TestNavigationTreeNode[] = [
        {
          id: 'a',
          label: 'a',
          level: 0,
          totalTests: 1,
          failedTests: 1,
          passedTests: 0,
          flakyTests: 0,
          skippedTests: 0,
          errorTests: 0,
          precludedTests: 0,
          unknownTests: 0,
          isStructured: true,
          children: [
            {
              id: 'b',
              label: 'b',
              level: 1,
              totalTests: 1,
              failedTests: 1,
              passedTests: 0,
              flakyTests: 0,
              skippedTests: 0,
              errorTests: 0,
              precludedTests: 0,
              unknownTests: 0,
              isStructured: true,
              children: [
                {
                  id: 'c',
                  label: 'c',
                  level: 2,
                  totalTests: 1,
                  failedTests: 1,
                  passedTests: 0,
                  flakyTests: 0,
                  skippedTests: 0,
                  errorTests: 0,
                  precludedTests: 0,
                  unknownTests: 0,
                  testVariant: {} as TestVariant,
                  isStructured: true,
                },
              ],
            },
          ],
        },
      ];
      const compressed = compressSingleChildNodes(tree);
      expect(compressed).toHaveLength(1);
      expect(compressed[0].label).toBe('ab');
      expect(compressed[0].children).toHaveLength(1);
      expect(compressed[0].children?.[0].label).toBe('c');
      expect(compressed[0].children?.[0].testVariant).toBeDefined();
    });

    it('should not compress nodes with multiple children', () => {
      const tree: TestNavigationTreeNode[] = [
        {
          id: 'a',
          label: 'a',
          level: 0,
          totalTests: 2,
          failedTests: 1,
          passedTests: 1,
          flakyTests: 0,
          skippedTests: 0,
          errorTests: 0,
          precludedTests: 0,
          unknownTests: 0,
          isStructured: true,

          children: [
            {
              id: 'b',
              label: 'b',
              level: 1,
              totalTests: 1,
              failedTests: 1,
              passedTests: 0,
              flakyTests: 0,
              skippedTests: 0,
              errorTests: 0,
              precludedTests: 0,
              unknownTests: 0,
              testVariant: {} as TestVariant,
              isStructured: true,
            },
            {
              id: 'c',
              label: 'c',
              level: 1,
              totalTests: 1,
              failedTests: 0,
              passedTests: 1,
              flakyTests: 0,
              skippedTests: 0,
              errorTests: 0,
              precludedTests: 0,
              unknownTests: 0,
              testVariant: {} as TestVariant,
              isStructured: true,
            },
          ],
        },
      ];
      const compressed = compressSingleChildNodes(tree);
      expect(compressed).toEqual(tree); // No change
    });

    it('should not compress nodes with multiple children and different test variants', () => {
      const tree: TestNavigationTreeNode[] = [
        {
          id: 'a',
          label: 'a',
          level: 0,
          totalTests: 2,
          failedTests: 1,
          passedTests: 1,
          flakyTests: 2,
          skippedTests: 3,
          errorTests: 0,
          precludedTests: 3,
          unknownTests: 0,
          isStructured: true,
          children: [
            {
              id: 'b',
              label: 'b',
              level: 1,
              totalTests: 1,
              failedTests: 1,
              passedTests: 0,
              flakyTests: 1,
              skippedTests: 3,
              errorTests: 0,
              precludedTests: 1,
              unknownTests: 0,
              testVariant: {} as TestVariant,
              isStructured: true,
            },
            {
              id: 'c',
              label: 'c',
              level: 1,
              totalTests: 1,
              failedTests: 0,
              passedTests: 1,
              flakyTests: 1,
              skippedTests: 0,
              errorTests: 0,
              precludedTests: 2,
              unknownTests: 0,
              testVariant: {} as TestVariant,
              isStructured: true,
            },
          ],
        },
      ];
      const compressed = compressSingleChildNodes(tree);
      expect(compressed).toEqual(tree); // No change
    });
  });

  describe('buildFlatTree', () => {
    it('should build a tree from flat test IDs', () => {
      const variants: TestVariant[] = [
        { testId: 'a/b/c', statusV2: TestVerdict_Status.FAILED },
        { testId: 'a/b/d', statusV2: TestVerdict_Status.PASSED },
        { testId: 'a/e', statusV2: TestVerdict_Status.FAILED },
      ] as TestVariant[];

      const tree = buildFlatTree(variants);

      expect(tree).toHaveLength(1);
      const nodeA = tree[0];
      expect(nodeA.label).toBe('a/');
      expect(nodeA.totalTests).toBe(3);
      expect(nodeA.failedTests).toBe(2);
      expect(nodeA.children).toHaveLength(2);

      const nodeB = nodeA.children![0];
      expect(nodeB.label).toBe('b/');
      expect(nodeB.totalTests).toBe(2);
      expect(nodeB.failedTests).toBe(1);
      expect(nodeB.children).toHaveLength(2);

      const nodeC = nodeB.children![0];
      expect(nodeC.label).toBe('c');
      expect(nodeC.totalTests).toBe(1);
      expect(nodeC.failedTests).toBe(1);
      expect(nodeC.children).toBeUndefined();

      const nodeD = nodeB.children![1];
      expect(nodeD.label).toBe('d');
      expect(nodeD.totalTests).toBe(1);
      expect(nodeD.failedTests).toBe(0);
      expect(nodeD.children).toBeUndefined();

      const nodeE = nodeA.children![1];
      expect(nodeE.label).toBe('e');
      expect(nodeE.totalTests).toBe(1);
      expect(nodeE.failedTests).toBe(1);
      expect(nodeE.children).toBeUndefined();
    });
  });

  describe('buildFailureReasonTree', () => {
    it('should group test variants by normalized failure reason', () => {
      const reason1a = 'reason with number 12345';
      const reason1b = 'reason with number 67890';
      const reason2 = 'another reason';
      const normalizedReason1 = normalizeDrawerFailureReason(reason1a);

      const variants: TestVariant[] = [
        {
          testId: 'test1',
          results: [
            {
              result: {
                statusV2: TestResult_Status.FAILED,
                failureReason: { primaryErrorMessage: reason1a },
              },
            },
          ],
        },
        {
          testId: 'test2',
          results: [
            {
              result: {
                statusV2: TestResult_Status.FAILED,
                failureReason: { primaryErrorMessage: reason2 },
              },
            },
          ],
        },
        {
          testId: 'test3',
          results: [
            {
              result: {
                statusV2: TestResult_Status.FAILED,
                failureReason: { primaryErrorMessage: reason1b },
              },
            },
          ],
        },
        {
          testId: 'test4',
          results: [{ result: { statusV2: TestResult_Status.PASSED } }],
        },
      ] as unknown as TestVariant[];

      const treeGroups = buildFailureReasonTree(variants);

      expect(treeGroups).toHaveLength(2);

      const group1 = treeGroups.find((g) => g.label === normalizedReason1);
      expect(group1).toBeDefined();
      expect(group1!.nodes).toHaveLength(2); // test1 and test3
      expect(group1!.totalTests).toBe(2);

      const group2 = treeGroups.find((g) => g.label === reason2);
      expect(group2).toBeDefined();
      expect(group2!.nodes).toHaveLength(1); // test2
      expect(group2!.totalTests).toBe(1);
    });
  });

  describe('buildHierarchyTreeAndFindExpandedIds', () => {
    const structuredVariant1: TestVariant = {
      testId: 'test.structured.case1',
      variantHash: 'hash1',
      testIdStructured: {
        moduleName: 'module1',
        moduleScheme: 'go',
        moduleVariantHash: 'vhash1',
        coarseName: 'coarse1',
        fineName: 'fine1',
        caseName: 'case1',
      },
    } as TestVariant;

    const structuredVariant2: TestVariant = {
      testId: 'test.structured.case2',
      variantHash: 'hash2',
      testIdStructured: {
        moduleName: 'module1',
        moduleScheme: 'go',
        moduleVariantHash: 'vhash1',
        coarseName: 'coarse1',
        fineName: 'fine1',
        caseName: 'case2',
      },
    } as TestVariant;

    const flatVariant1: TestVariant = {
      testId: 'a/b/c',
      variantHash: 'hashA',
    } as TestVariant;
    const flatVariant2: TestVariant = {
      testId: 'a/b/d',
      variantHash: 'hashB',
    } as TestVariant;

    const compressedVariant: TestVariant = {
      testId: 'long/path/to/single/file',
      variantHash: 'hashComp',
    } as TestVariant;

    it('should return the correct path for a structured variant', () => {
      const variants = [structuredVariant1, structuredVariant2];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'test.structured.case2',
        'hash2',
      );

      expect(idsToExpand).toEqual([
        '0-module1',
        '0-module1/1-vhash1',
        '0-module1/1-vhash1/2-coarse1',
        '0-module1/1-vhash1/2-coarse1/3-fine1',
        '0-module1/1-vhash1/2-coarse1/3-fine1/4-case2',
      ]);
    });

    it('should return the correct path for a flat variant', () => {
      const variants = [flatVariant1, flatVariant2];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'a/b/d',
        'hashB',
      );

      expect(idsToExpand).toEqual(['a/-a/b/', 'a/b/d']);
    });

    it('should return the correct path for a compressed flat variant', () => {
      const variants = [compressedVariant];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'long/path/to/single/file',
        'hashComp',
      );

      expect(idsToExpand).toEqual([
        'long/-long/path/-long/path/to/-long/path/to/single/',
        'long/path/to/single/file',
      ]);
    });

    it('should return the correct path from a mixed list (structured)', () => {
      const variants = [
        structuredVariant1,
        structuredVariant2,
        flatVariant1,
        flatVariant2,
      ];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'test.structured.case1',
        'hash1',
      );

      expect(idsToExpand).toEqual([
        '0-module1',
        '0-module1/1-vhash1',
        '0-module1/1-vhash1/2-coarse1',
        '0-module1/1-vhash1/2-coarse1/3-fine1',
        '0-module1/1-vhash1/2-coarse1/3-fine1/4-case1',
      ]);
    });

    it('should return the correct path from a mixed list (flat)', () => {
      const variants = [
        structuredVariant1,
        structuredVariant2,
        flatVariant1,
        flatVariant2,
      ];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'a/b/c',
        'hashA',
      );

      expect(idsToExpand).toEqual(['a/-a/b/', 'a/b/c']);
    });

    it('should return an empty array if no test ID is provided', () => {
      const variants = [structuredVariant1, flatVariant1];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(variants);
      expect(idsToExpand).toEqual([]);
    });

    it('should return an empty array if the test ID is not found', () => {
      const variants = [structuredVariant1, flatVariant1];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'non/existent/id',
        'hashNotFound',
      );
      expect(idsToExpand).toEqual([]);
    });

    it('should return an empty array if the variant hash is not found', () => {
      const variants = [structuredVariant1, flatVariant1];
      const { idsToExpand } = buildHierarchyTreeAndFindExpandedIds(
        variants,
        'a/b/c',
        'wrongHash',
      );
      expect(idsToExpand).toEqual([]);
    });
  });
});
