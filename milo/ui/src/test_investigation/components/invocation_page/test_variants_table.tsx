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

import { Box, Chip, Link } from '@mui/material';
import { useMemo, useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router';

import {
  getStatusStyle,
  semanticStatusForTestVariant,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import {
  CategoryOption,
  MultiSelectCategoryChip,
} from '@/generic_libs/components/filter';
import { FilterBarContainer } from '@/generic_libs/components/filter/filter_bar_container';
import {
  ColumnDefinition,
  RowData,
  TreeTable,
} from '@/generic_libs/components/table';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { TestNavigationTreeNode } from '@/test_investigation/components/test_navigation_drawer/types';
import {
  buildHierarchyTree,
  HierarchyBuildResult,
} from '@/test_investigation/utils/drawer_tree_utils';

/**
 * Recursively traverses the node tree to find the IDs of all parent nodes
 * that contain at least one failed test in their hierarchy.
 * @param nodes The nodes to search through.
 * @returns A flat array of node IDs to be expanded.
 */
function getIdsOfAllNodes(nodes: TestNavigationTreeNode[]): string[] {
  let ids: string[] = [];
  for (const node of nodes) {
    if (node.children?.length) {
      ids.push(node.id);
      ids = ids.concat(getIdsOfAllNodes(node.children));
    }
  }
  return ids;
}

/**
 * Recursively filters the tree data based on a set of selected statuses.
 * A parent node is kept if any of its descendants are a match.
 * @param nodes The tree nodes to filter.
 * @param statuses The set of statuses to keep. If empty, all nodes are returned.
 * @returns The filtered array of nodes.
 */
function filterTreeByStatus(
  nodes: readonly TestNavigationTreeNode[],
  statuses: Set<string>,
): TestNavigationTreeNode[] {
  // TODO: this filtering should be done on the backend, not here.

  // If no statuses are selected, no filtering is needed.
  if (statuses.size === 0) {
    return [...nodes];
  }

  const filteredNodes: TestNavigationTreeNode[] = [];

  for (const node of nodes) {
    // If the node is a parent, filter its children recursively.
    if (node.children?.length) {
      const filteredChildren = filterTreeByStatus(node.children, statuses);
      // Keep the parent if any of its children matched the filter.
      if (filteredChildren.length > 0) {
        filteredNodes.push({ ...node, children: filteredChildren });
      }
    } else if (node.testVariant) {
      // If the node is a leaf, check if its status is in the selected set.
      if (statuses.has(semanticStatusForTestVariant(node.testVariant))) {
        filteredNodes.push(node);
      }
    }
  }

  return filteredNodes;
}

/**
 * Renders a tree table view of test variants grouped by test ID hierarchy.
 * It supports filtering by test variant status and navigating to individual
 * test variant pages.
 */
export function TestVariantsTable({
  testVariants,
}: {
  testVariants: readonly TestVariant[];
}) {
  const { invocationId } = useParams<{ invocationId: string }>();
  const navigate = useNavigate();
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [selectedStatuses, setSelectedStatuses] = useState<
    Set<SemanticStatusType>
  >(new Set(['failed', 'execution_errored']));

  const { tree: hierarchyTreeData } = useMemo(
    (): HierarchyBuildResult => buildHierarchyTree(testVariants),
    [testVariants],
  );

  const filteredHierarchyTreeData = useMemo(
    () => filterTreeByStatus(hierarchyTreeData, selectedStatuses),
    [hierarchyTreeData, selectedStatuses],
  );

  const handleSelectTestVariant = useCallback(
    (testId: string, variantHash: string) => {
      if (invocationId && testId && variantHash) {
        navigate(
          `/ui/test-investigate/invocations/${invocationId}/tests/${encodeURIComponent(
            testId,
          )}/variants/${variantHash}`,
        );
      }
    },
    [invocationId, navigate],
  );
  const columns: ColumnDefinition[] = useMemo(() => {
    return [
      {
        id: 'label',
        label: 'Test',
        width: '350px',
        // Add a custom cell renderer to make leaf nodes clickable.
        renderCell: (data: RowData) => {
          const rowData = data as TestNavigationTreeNode;
          const isLeafWithVariant = rowData.testVariant;

          if (isLeafWithVariant) {
            const testVariant = rowData.testVariant!;
            const semanticStatus = semanticStatusForTestVariant(testVariant);
            const styles = getStatusStyle(semanticStatus);
            const IconComponent = styles.icon;

            return (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {IconComponent && (
                  <IconComponent
                    sx={{
                      fontSize: '18px',
                      color: styles.iconColor,
                    }}
                  />
                )}
                <Link
                  component="button"
                  variant="body2"
                  onClick={() =>
                    handleSelectTestVariant(
                      testVariant.testId,
                      testVariant.variantHash,
                    )
                  }
                  sx={{
                    textAlign: 'left',
                    textTransform: 'none',
                    textDecoration: 'none',
                    color: styles.textColor,
                  }}
                >
                  {rowData.label}
                </Link>
              </Box>
            );
          } else {
            return (
              <>
                {rowData.label}{' '}
                <Chip color="error" label={`${rowData.failedTests} failed`} />
              </>
            );
          }
        },
      },
    ];
  }, [handleSelectTestVariant]);

  useEffect(() => {
    if (filteredHierarchyTreeData.length > 0) {
      const idsToExpand = getIdsOfAllNodes(filteredHierarchyTreeData);
      setExpandedNodes(new Set(idsToExpand));
    } else {
      // Clear expansion if filter results are empty.
      setExpandedNodes(new Set());
    }
  }, [filteredHierarchyTreeData]);

  const rows = filteredHierarchyTreeData;

  const statusFilterOptions: CategoryOption[] = useMemo(() => {
    return [
      { label: 'Failed', value: 'failed' },
      { label: 'Execution Errored', value: 'execution_errored' },
      { label: 'Exonerated', value: 'exonerated' },
      { label: 'Flaky', value: 'flaky' },
      { label: 'Precluded', value: 'precluded' },
      { label: 'Passed', value: 'passed' },
      { label: 'Skipped', value: 'skipped' },
    ];
  }, []);

  return (
    <TreeTable
      data={rows}
      columns={columns}
      expandedRowIds={expandedNodes}
      onExpandedRowIdsChange={setExpandedNodes}
      headerChildren={
        <FilterBarContainer
          showClearAll={selectedStatuses.size > 0}
          onClearAll={() => setSelectedStatuses(new Set())}
        >
          {/* TODO: Probably want a chip per status here.  Check with UX. */}
          <MultiSelectCategoryChip
            categoryName="Test Status"
            availableOptions={statusFilterOptions}
            selectedItems={selectedStatuses}
            onSelectedItemsChange={(value) =>
              setSelectedStatuses(value as Set<SemanticStatusType>)
            }
          />
        </FilterBarContainer>
      }
      placeholder={
        selectedStatuses.size > 0
          ? 'No tests match the applied filters.  Please remove the filters to see any available tests.'
          : 'All tests in the invocation passed.'
      }
    />
  );
}
