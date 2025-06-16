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
  AppliedFilterChip,
  MultiSelectCategoryChip,
} from '@/generic_libs/components/filter';
import { FilterBarContainer } from '@/generic_libs/components/filter/filter_bar_container';
import {
  ColumnDefinition,
  RowData,
  TreeTable,
} from '@/generic_libs/components/table';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { TestNavigationTreeNode } from '@/test_investigation/components/test_navigation_drawer/types';

/**
 * Recursively traverses the node tree to find the IDs of all nodes.
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

interface TestVariantsTableProps {
  treeData: TestNavigationTreeNode[];
  parsedTestId: string | null;
  parsedVariantDef: Readonly<Record<string, string>> | null;
  selectedStatuses: Set<SemanticStatusType>;
  setSelectedStatuses: (newSelection: Set<SemanticStatusType>) => void;
}

/**
 * Renders a tree table view of test variants grouped by test ID hierarchy.
 * It supports filtering by test variant status and navigating to individual
 * test variant pages.
 */
export function TestVariantsTable({
  treeData,
  parsedTestId,
  parsedVariantDef,
  selectedStatuses,
  setSelectedStatuses,
}: TestVariantsTableProps) {
  const { invocationId } = useParams<{ invocationId: string }>();
  const navigate = useNavigate();
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [_, setSearchParams] = useSyncedSearchParams();
  const filteredHierarchyTreeData = treeData;

  const handleRemoveTestIdFilter = useCallback(() => {
    setSearchParams((prev) => {
      const newParams = new URLSearchParams(prev);
      newParams.delete('testId');
      return newParams;
    });
  }, [setSearchParams]);

  const handleRemoveVariantFilter = useCallback(
    (keyToRemove: string) => {
      if (!parsedVariantDef) return;

      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        const valueToRemove = `${keyToRemove}:${parsedVariantDef[keyToRemove]}`;

        const allVariants = newParams.getAll('v');
        const newVariants = allVariants.filter((v) => v !== valueToRemove);

        newParams.delete('v');
        newVariants.forEach((v) => newParams.append('v', v));

        return newParams;
      });
    },
    [setSearchParams, parsedVariantDef],
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
    const idsToExpand = getIdsOfAllNodes(filteredHierarchyTreeData);
    setExpandedNodes(new Set(idsToExpand));
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

  const hasActiveFilters =
    parsedTestId ||
    (parsedVariantDef && Object.keys(parsedVariantDef).length > 0);

  return (
    <TreeTable
      data={rows}
      columns={columns}
      expandedRowIds={expandedNodes}
      onExpandedRowIdsChange={setExpandedNodes}
      headerChildren={
        <FilterBarContainer
          showClearAll={
            selectedStatuses.size > 0 || !!parsedTestId || !!parsedVariantDef
          }
          onClearAll={() => {
            setSelectedStatuses(new Set());
            if (parsedTestId) {
              handleRemoveTestIdFilter();
            }
            if (parsedVariantDef) {
              Object.keys(parsedVariantDef).forEach(handleRemoveVariantFilter);
            }
          }}
        >
          {/* Render chips for active Test ID and Variant filters. */}
          {parsedTestId && (
            <AppliedFilterChip
              filterKey="Test ID"
              filterValue={parsedTestId}
              onRemove={handleRemoveTestIdFilter}
            />
          )}
          {parsedVariantDef &&
            Object.entries(parsedVariantDef).map(([key, value]) => (
              <AppliedFilterChip
                key={key}
                filterKey={key}
                filterValue={value}
                onRemove={() => handleRemoveVariantFilter(key)}
              />
            ))}
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
        selectedStatuses.size > 0 || hasActiveFilters
          ? 'No tests match the applied filters. Please remove the filters to see any available tests.'
          : 'All tests in the invocation passed.'
      }
    />
  );
}
