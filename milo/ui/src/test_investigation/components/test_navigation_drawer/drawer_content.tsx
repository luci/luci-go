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
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import React from 'react';

import { DrawerTreeItem } from './drawer_tree_item';
import { DrawerTreeNode } from './types';

interface DrawerContentProps {
  selectedTab: number;
  onTabChange: (event: React.SyntheticEvent, newValue: number) => void;
  isLoadingTestVariants: boolean;
  hierarchyTreeData: readonly DrawerTreeNode[];
  failureReasonTreeData: readonly DrawerTreeNode[];
  expandedNodes: Set<string>;
  toggleNodeExpansion: (nodeId: string) => void;
  currentTestId?: string;
  currentVariantHash?: string;
  onSelectTestVariant?: (testId: string, variantHash: string) => void;
  closeDrawer?: () => void;
}

export function DrawerContent({
  selectedTab,
  onTabChange,
  isLoadingTestVariants,
  hierarchyTreeData,
  failureReasonTreeData,
  expandedNodes,
  toggleNodeExpansion,
  currentTestId,
  currentVariantHash,
  onSelectTestVariant,
  closeDrawer,
}: DrawerContentProps): JSX.Element {
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
      <Typography
        variant="h6"
        sx={{
          p: 2,
          flexShrink: 0,
          borderBottom: 1,
          borderColor: 'divider',
          fontWeight: 500,
        }}
      >
        Platform Testing Infra
      </Typography>
      <Tabs
        value={selectedTab}
        onChange={onTabChange}
        variant="fullWidth"
        sx={{ flexShrink: 0, borderBottom: 1, borderColor: 'divider' }}
      >
        <Tab
          label="Test hierarchy"
          sx={{ textTransform: 'none', fontSize: '0.875rem' }}
        />
        <Tab
          label="Failure reason"
          sx={{ textTransform: 'none', fontSize: '0.875rem' }}
        />
      </Tabs>
      <Box sx={{ flexGrow: 1, overflowY: 'auto' }}>
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
          <List dense component="nav">
            {selectedTab === 0 &&
              (hierarchyTreeData.length > 0 ? (
                hierarchyTreeData.map((node) => (
                  <DrawerTreeItem
                    key={node.id}
                    node={node}
                    expandedNodes={expandedNodes}
                    toggleNodeExpansion={toggleNodeExpansion}
                    currentTestId={currentTestId}
                    currentVariantHash={currentVariantHash}
                    onSelectTestVariant={onSelectTestVariant}
                    closeDrawer={closeDrawer}
                  />
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
                failureReasonTreeData.map((node) => (
                  <DrawerTreeItem
                    key={node.id}
                    node={node}
                    expandedNodes={expandedNodes}
                    toggleNodeExpansion={toggleNodeExpansion}
                    currentTestId={currentTestId}
                    currentVariantHash={currentVariantHash}
                    onSelectTestVariant={undefined} // TODO: Failure reasons aren't directly navigable
                    closeDrawer={closeDrawer}
                  />
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
