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

import { List } from '@mui/material';
import { JSX } from 'react';

import { ExpandableListItem } from './expandable_list_item.tsx';
import { TestNavigationTreeNode } from './types';

interface DrawerTreeItemProps {
  node: TestNavigationTreeNode;
  expandedNodes: Set<string>;
  toggleNodeExpansion: (nodeId: string) => void;
  currentTestId?: string;
  currentVariantHash?: string;
  onSelectTestVariant?: (testId: string, variantHash: string) => void;
}

export function DrawerTreeItem({
  node,
  expandedNodes,
  toggleNodeExpansion,
  currentTestId,
  currentVariantHash,
  onSelectTestVariant,
}: DrawerTreeItemProps): JSX.Element {
  const isExpanded = expandedNodes.has(node.id);
  const hasChildren = node.children && node.children.length > 0;

  const handleItemClick = () => {
    if (hasChildren) {
      toggleNodeExpansion(node.id);
    } else if (node.testVariant !== undefined && onSelectTestVariant) {
      onSelectTestVariant(
        node.testVariant.testId,
        node.testVariant.variantHash,
      );
    }
  };

  const getSecondaryText = () => {
    if (node.failedTests === undefined || node.totalTests === undefined) {
      return undefined;
    }
    if (hasChildren) {
      return `${node.failedTests || 0} failed (${node.totalTests || 0} total)`;
    } else {
      if (node.failedTests === 0) {
        return 'Passed';
      } else {
        return 'Failed';
      }
    }
  };

  return (
    <ExpandableListItem
      isExpanded={isExpanded}
      label={node.label}
      secondaryText={getSecondaryText()}
      onClick={handleItemClick}
      isSelected={
        currentTestId === node.testVariant?.testId &&
        currentVariantHash === node.testVariant?.variantHash
      }
      level={node.level}
    >
      {hasChildren && (
        <List component="div" disablePadding dense>
          {node.children!.map((child) => (
            <DrawerTreeItem
              key={child.id}
              node={child}
              expandedNodes={expandedNodes}
              toggleNodeExpansion={toggleNodeExpansion}
              currentTestId={currentTestId}
              currentVariantHash={currentVariantHash}
              onSelectTestVariant={onSelectTestVariant}
            />
          ))}
        </List>
      )}
    </ExpandableListItem>
  );
}
