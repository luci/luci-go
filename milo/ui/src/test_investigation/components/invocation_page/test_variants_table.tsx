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
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Box,
  Chip,
  Link,
  CircularProgress,
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Typography,
} from '@mui/material';
import createDomPurify from 'dompurify';
import { useMemo, useCallback, useEffect, useState } from 'react';
import { useParams } from 'react-router';

import {
  getStatusStyle,
  semanticStatusForTestVariant,
  SemanticStatusType,
} from '@/common/styles/status_styles';
import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import {
  CategoryOption,
  AppliedFilterChip,
  MultiSelectCategoryChip,
  TextInputFilterChip,
} from '@/generic_libs/components/filter';
import { FilterBarContainer } from '@/generic_libs/components/filter/filter_bar_container';
import {
  ColumnDefinition,
  RowData,
  VirtualTreeTable,
} from '@/generic_libs/components/table';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  TestResult,
  TestResult_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestResultBundle } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
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
  isLoading: boolean;
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
  isLoading,
  parsedTestId,
  parsedVariantDef,
  selectedStatuses,
  setSelectedStatuses,
}: TestVariantsTableProps) {
  const { invocationId } = useParams<{ invocationId: string }>();
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set());
  const [_, setSearchParams] = useSyncedSearchParams();
  const filteredHierarchyTreeData = treeData;
  const domPurify = useMemo(() => {
    return createDomPurify(window);
  }, []);

  const handleTestIdChange = useCallback(
    (testId: string | null) => {
      setSearchParams((prev) => {
        const newParams = new URLSearchParams(prev);
        if (testId) {
          newParams.set('testId', testId);
        } else {
          newParams.delete('testId');
        }
        return newParams;
      });
    },
    [setSearchParams],
  );

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

  const getTestVariantURL = useCallback(
    (testId: string, variantHash: string) => {
      if (invocationId && testId && variantHash) {
        return `/ui/test-investigate/invocations/${invocationId}/tests/${encodeURIComponent(
          testId,
        )}/variants/${variantHash}`;
      }
      return '#';
    },
    [invocationId],
  );

  const getFailureSummary = useCallback(
    (testVariant: TestVariant) => {
      const resultsBundle = testVariant.results as TestResultBundle[];
      const results: TestResult[] = resultsBundle
        .filter((resultsBundle) => resultsBundle.result !== undefined)
        .map((resultsBundle) => resultsBundle.result!);
      if (!results || results.length === 0) {
        return undefined;
      }

      // Prioritise displaying failed -> skipped -> execution errored test results.
      const statusPriorityMap = {
        [TestResult_Status.FAILED]: 0,
        [TestResult_Status.SKIPPED]: 1,
        [TestResult_Status.EXECUTION_ERRORED]: 2,
        [TestResult_Status.PASSED]: 3,
        [TestResult_Status.PRECLUDED]: 4,
        [TestResult_Status.STATUS_UNSPECIFIED]: 5,
      };

      results.sort((a, b) => {
        return statusPriorityMap[a.statusV2] - statusPriorityMap[b.statusV2];
      });

      const resultToDisplay = results[0];
      // Show in priority of failure or skipped reason -> stack trace -> summary_html.
      const summary =
        resultToDisplay.failureReason?.primaryErrorMessage ||
        resultToDisplay.failureReason?.errors?.[0]?.message ||
        resultToDisplay?.skippedReason?.reasonMessage;

      if (summary) {
        return summary;
      }
      // If none of the above, display summary_html.
      if (resultToDisplay.summaryHtml) {
        const innerText = domPurify.sanitize(resultToDisplay.summaryHtml, {
          ALLOWED_TAGS: ['#text'],
        });
        return innerText;
      }
      // TODO: fetch text artifact content also.
      return undefined;
    },
    [domPurify],
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
          const isTestIdStructured = rowData.isStructured;

          if (isLeafWithVariant) {
            const testVariant = rowData.testVariant!;
            const semanticStatus = semanticStatusForTestVariant(testVariant);
            const styles = getStatusStyle(semanticStatus);
            const IconComponent = styles.icon;

            const testTextToCopy = isTestIdStructured
              ? rowData.label
              : testVariant.testId;

            const failureSummary = getFailureSummary(testVariant);
            return (
              <>
                {failureSummary ? (
                  <Accordion
                    sx={{
                      p: 0,
                      border: 'none',
                      backgroundColor: 'rgba(0, 0, 255, 0)',
                    }}
                  >
                    <AccordionSummary
                      expandIcon={<ExpandMoreIcon />}
                      sx={{
                        p: 0,
                        flexDirection: 'row-reverse',
                      }}
                    >
                      <Box
                        sx={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 0.5,
                          p: 0,
                        }}
                      >
                        {IconComponent && (
                          <IconComponent
                            sx={{
                              fontSize: '18px',
                              color: styles.iconColor,
                            }}
                          />
                        )}
                        <Box
                          sx={{
                            display: 'flex',
                            alignItems: 'center',
                            maxWidth: '82vw',
                          }}
                        >
                          <Link
                            href={getTestVariantURL(
                              testVariant.testId,
                              testVariant.variantHash,
                            )}
                            variant="body2"
                            sx={{
                              textAlign: 'left',
                              textTransform: 'none',
                              textDecoration: 'none',
                              color: styles.textColor,
                            }}
                            onClick={(e) => {
                              {
                                e.stopPropagation();
                              }
                            }}
                          >
                            {rowData.label}
                          </Link>
                          <CopyToClipboard
                            textToCopy={testTextToCopy}
                            aria-label="Copy text."
                            sx={{ ml: 0.5, minWidth: '18px' }}
                          />
                          <Typography
                            variant="caption"
                            sx={{
                              p: 1,
                              overflow: 'hidden',
                              textOverflow: 'ellipsis',
                              display: '-webkit-box',
                              WebkitLineClamp: '1',
                              whiteSpace: 'nowrap',
                              WebkitBoxOrient: 'vertical',
                              color: 'text.secondary',
                            }}
                          >
                            {failureSummary}
                          </Typography>
                        </Box>
                      </Box>
                    </AccordionSummary>
                    <AccordionDetails sx={{ p: 0 }}>
                      <Box sx={{ maxWidth: '90vw' }}>
                        <Typography
                          sx={{
                            p: 1,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            display: '-webkit-box',
                            WebkitLineClamp: '6',
                            WebkitBoxOrient: 'vertical',
                          }}
                          variant="caption"
                        >
                          {failureSummary}
                        </Typography>
                      </Box>
                    </AccordionDetails>
                  </Accordion>
                ) : (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.5,
                      p: 0,
                    }}
                  >
                    {IconComponent && (
                      <IconComponent
                        sx={{
                          fontSize: '18px',
                          color: styles.iconColor,
                        }}
                      />
                    )}
                    <Link
                      href={getTestVariantURL(
                        testVariant.testId,
                        testVariant.variantHash,
                      )}
                      variant="body2"
                      sx={{
                        textAlign: 'left',
                        textTransform: 'none',
                        textDecoration: 'none',
                        color: styles.textColor,
                      }}
                      onClick={(e) => {
                        {
                          e.stopPropagation();
                        }
                      }}
                    >
                      {rowData.label}
                    </Link>
                    <CopyToClipboard
                      textToCopy={testTextToCopy}
                      aria-label="Copy text."
                      sx={{ ml: 0.5 }}
                    />
                  </Box>
                )}
              </>
            );
          } else {
            return (
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                {rowData.label}{' '}
                <Chip color="error" label={`${rowData.failedTests} failed`} />
                {isTestIdStructured && (
                  <CopyToClipboard
                    textToCopy={rowData.label}
                    aria-label="Copy text."
                    sx={{ ml: 0.5 }}
                  />
                )}
              </Box>
            );
          }
        },
      },
    ];
  }, [getTestVariantURL, getFailureSummary]);

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
    <Box sx={{ position: 'relative' }}>
      {isLoading && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            width: '100%',
            height: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            backgroundColor: 'rgba(255, 255, 255, 0.5)',
            zIndex: 1,
          }}
        >
          <CircularProgress />
        </Box>
      )}
      <VirtualTreeTable
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
              handleTestIdChange(null);
              if (parsedVariantDef) {
                Object.keys(parsedVariantDef).forEach(
                  handleRemoveVariantFilter,
                );
              }
            }}
          >
            <TextInputFilterChip
              categoryName="Test ID"
              value={parsedTestId}
              onValueChange={handleTestIdChange}
            />
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
    </Box>
  );
}
