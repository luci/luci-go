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

import {
  getStatusStyle,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';

import { getSemanticStatusFromVerdict } from '../../utils/drawer_tree_utils';

import { ExpandableListItem } from './expandable_list_item.tsx';
import { TestNavigationTreeNode } from './types';

interface DrawerTreeItemProps {
  indent: number;
  node: TestNavigationTreeNode;
  expandedNodes: Set<string>;
  toggleNodeExpansion: (nodeId: string) => void;
  currentTestId?: string;
  currentVariantHash?: string;
  onSelectTestVariant?: (testId: string, variantHash: string) => void;
}

export function DrawerTreeItem({
  indent,
  node,
  expandedNodes,
  toggleNodeExpansion,
  currentTestId,
  currentVariantHash,
  onSelectTestVariant,
}: DrawerTreeItemProps): JSX.Element {
  const isExpanded = expandedNodes.has(node.id);
  const hasChildren = node.children && node.children.length > 0;

  let primarySemanticStatus: SemanticStatusType = 'neutral';
  if (node.testVariant !== undefined) {
    primarySemanticStatus = getSemanticStatusFromVerdict(
      node.testVariant.statusV2,
    );
  }

  const itemStyle: StatusStyle = getStatusStyle(primarySemanticStatus);
  const ItemIconComponent = itemStyle.icon;

  let nodeIconElement: JSX.Element | null = null;

  if (ItemIconComponent) {
    nodeIconElement = (
      <ItemIconComponent
        sx={{
          fontSize: 'inherit',
          color: itemStyle.iconColor || itemStyle.textColor,
        }}
      />
    );
  }

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

  return (
    <ExpandableListItem
      isExpanded={isExpanded}
      label={node.label}
      secondaryText={
        hasChildren &&
        (node.failedTests !== undefined || node.totalTests !== undefined)
          ? `${node.failedTests || 0} failed (${node.totalTests || 0} total)`
          : undefined
      }
      onClick={handleItemClick}
      isSelected={
        currentTestId === node.testVariant?.testId &&
        currentVariantHash === node.testVariant?.variantHash
      }
      level={node.level + indent}
      iconElement={
        !hasChildren && nodeIconElement ? nodeIconElement : undefined
      }
    >
      {hasChildren && (
        <List component="div" disablePadding dense>
          {node.children!.map((child) => (
            <DrawerTreeItem
              key={child.id}
              indent={indent}
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
