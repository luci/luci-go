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

import {
  Box,
  CircularProgress,
  List,
  Typography,
  ToggleButtonGroup,
  ToggleButton,
} from '@mui/material';
import { useState, useCallback, useEffect } from 'react';

import {
  TestVerdict_Status,
  testVerdict_StatusToJSON,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';

import { useTestDrawer } from './context';
import { DrawerTreeItem } from './drawer_tree_item';
import { ExpandableListItem } from './expandable_list_item';
import { TestNavigationTreeGroup } from './types';

export function DrawerContent() {
  const {
    isLoading: isLoadingTestVariants,
    hierarchyTree: hierarchyTreeData,
    failureReasonTree: failureReasonTreeData,
    selectedItemRef,
    isDrawerOpen,
  } = useTestDrawer();

  const [selectedTab, setSelectedTab] = useState(0);
  const [openGroups, setOpenGroups] = useState<{ [groupId: string]: boolean }>(
    {},
  );

  // This effect handles scrolling the selected item into view
  useEffect(() => {
    // Only scroll when:
    // 1. The drawer is fully open and visible.
    // 2. Loading is finished.
    // 3. The ref has been attached to the selected item.
    if (isDrawerOpen && !isLoadingTestVariants && selectedItemRef?.current) {
      // We use a short timeout to allow MUI's <Collapse> components
      // to finish their 'auto' transition *after* the expandedNodes
      // state has been updated and the item is rendered.
      const timer = setTimeout(() => {
        selectedItemRef.current?.scrollIntoView({
          behavior: 'smooth',
          block: 'center',
        });
      }, 150);

      return () => clearTimeout(timer);
    }
    return;
  }, [isDrawerOpen, isLoadingTestVariants, selectedItemRef]);

  const handleTabChange = useCallback(
    (_event: React.SyntheticEvent, newValue: number | null) => {
      if (newValue !== null) {
        setSelectedTab(newValue);
      }
    },
    [],
  );

  const toggleGroup = (groupId: string) => {
    setOpenGroups((prev) => ({ ...prev, [groupId]: !prev[groupId] }));
  };

  const getStatusTotalsForGroup = (group: TestNavigationTreeGroup) => {
    const testStatusTotals = {
      [TestVerdict_Status.FAILED]: group.failedTests,
      [TestVerdict_Status.EXECUTION_ERRORED]: group.errorTests,
      [TestVerdict_Status.PRECLUDED]: group.precludedTests,
      [TestVerdict_Status.FLAKY]: group.flakyTests,
      [TestVerdict_Status.SKIPPED]: group.skippedTests,
      [TestVerdict_Status.PASSED]: group.passedTests,
      [TestVerdict_Status.STATUS_UNSPECIFIED]: group.unknownTests,
    };
    const countsArray: { type: TestVerdict_Status; value: number }[] = [];

    for (const [key, count] of Object.entries(testStatusTotals)) {
      const testVerdict: TestVerdict_Status | undefined = parseInt(key);
      countsArray.push({ type: testVerdict, value: count });
    }
    countsArray.sort((a, b) => b.value - a.value);
    const secondHighestCount =
      countsArray[1].value === 0
        ? ''
        : `, ${countsArray[1].value} ${testVerdict_StatusToJSON(countsArray[1].type)}`;
    const secondaryText = `${countsArray[0].value} ${testVerdict_StatusToJSON(countsArray[0].type)}${secondHighestCount}`;

    return { status: countsArray[0].type, secondaryText: secondaryText };
  };

  return (
    <Box
      sx={{
        width: '100%',
        overflow: 'hidden',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Box>
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            padding: 1.5,
            borderBottom: 1,
            borderColor: 'divider',
          }}
        >
          <Typography variant="subtitle1" sx={{ mr: 1 }}>
            Group by
          </Typography>
          <ToggleButtonGroup
            color="primary"
            value={selectedTab}
            exclusive
            size="small"
            onChange={handleTabChange}
            aria-label="Grouping options"
          >
            <ToggleButton
              value={0}
              aria-label="Test hierarchy"
              sx={{ textTransform: 'none', fontSize: '1rem' }}
            >
              Test hierarchy
            </ToggleButton>
            <ToggleButton
              value={1}
              aria-label="Failure reason"
              sx={{ textTransform: 'none', fontSize: '1rem' }}
            >
              Failure reason
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>
      </Box>

      <Box sx={{ flexGrow: 1, overflowY: 'auto', pt: 0 }}>
        {isLoadingTestVariants ? (
          <Box
            sx={{
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              p: 2,
              height: '100%',
            }}
          >
            <CircularProgress size={24} />
            <Typography sx={{ ml: 1 }}>Loading tests...</Typography>
          </Box>
        ) : (
          <List dense component="nav" disablePadding>
            {selectedTab === 0 &&
              (hierarchyTreeData.length > 0 ? (
                hierarchyTreeData.map((node) => (
                  <DrawerTreeItem key={node.id} node={node} />
                ))
              ) : (
                <Typography
                  sx={{ p: 2, textAlign: 'center', color: 'text.secondary' }}
                >
                  No tests found or structured hierarchy could not be built.
                </Typography>
              ))}
            {selectedTab === 1 &&
              (failureReasonTreeData.length > 0 ? (
                failureReasonTreeData.map((group) => (
                  <ExpandableListItem
                    key={group.id}
                    isExpanded={!!openGroups[group.id]}
                    label={group.label}
                    onClick={() => {
                      toggleGroup(group.id);
                    }}
                    totalTests={group.totalTests}
                    status={getStatusTotalsForGroup(group).status}
                    secondaryText={getStatusTotalsForGroup(group).secondaryText}
                  >
                    <List dense component="div" disablePadding>
                      {group.nodes.map((node) => (
                        <DrawerTreeItem key={node.id} node={node} />
                      ))}
                    </List>
                  </ExpandableListItem>
                ))
              ) : (
                <Typography
                  sx={{ p: 2, textAlign: 'center', color: 'text.secondary' }}
                >
                  No failures found to group by reason.
                </Typography>
              ))}
          </List>
        )}
      </Box>
    </Box>
  );
}
