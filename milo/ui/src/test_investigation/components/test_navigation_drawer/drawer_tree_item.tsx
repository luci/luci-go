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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import DescriptionIcon from '@mui/icons-material/Description';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FolderIcon from '@mui/icons-material/Folder';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import {
  Box,
  Chip,
  Collapse,
  IconButton,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
} from '@mui/material';
import React from 'react';

import {
  getStatusStyle,
  SemanticStatusType,
  StatusStyle,
} from '@/common/styles/status_styles';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { getSemanticStatusFromResultV2 } from '../../utils/drawer_tree_utils';

import { DrawerTreeNode } from './types';

interface DrawerTreeItemProps {
  node: DrawerTreeNode;
  expandedNodes: Set<string>;
  toggleNodeExpansion: (nodeId: string) => void;
  currentTestId?: string;
  currentVariantHash?: string;
  onSelectTestVariant?: (testId: string, variantHash: string) => void;
  closeDrawer?: () => void;
}

export function DrawerTreeItem({
  node,
  expandedNodes,
  toggleNodeExpansion,
  currentTestId,
  currentVariantHash,
  onSelectTestVariant,
  closeDrawer,
}: DrawerTreeItemProps): JSX.Element {
  const isExpanded = expandedNodes.has(node.id);
  const hasChildren = node.children && node.children.length > 0;

  // Determine the primary semantic status for the node's icon
  let primarySemanticStatus: SemanticStatusType = 'neutral';
  if (node.isLeaf && node.isClickable && node.status !== undefined) {
    // This is a test variant leaf node, its status comes from TestResult_Status
    primarySemanticStatus = getSemanticStatusFromResultV2(
      node.status as TestResult_Status,
    );
  } else if (!node.isLeaf && hasChildren) {
    // Folder node: derive status based on children, e.g., if any child is an error
    const hasErrorChild = node.children?.some(
      (childNode) =>
        childNode.isLeaf &&
        childNode.isClickable &&
        childNode.status === TestResult_Status.FAILED,
    );
    if (hasErrorChild) {
      primarySemanticStatus = 'error';
    } else {
      primarySemanticStatus = 'neutral'; // Default folder status
    }
  } else if (node.isLeaf && !node.isClickable && node.tagColor) {
    // For non-clickable leaves like failure reasons, use their tagColor as the primary semantic status
    primarySemanticStatus = node.tagColor;
  }

  const itemStyle: StatusStyle = getStatusStyle(primarySemanticStatus);
  const ItemIconComponent = itemStyle.icon;

  let nodeIconElement: JSX.Element | null = null;

  if (!node.isLeaf && hasChildren) {
    // Folder node
    const FolderSpecificIcon = isExpanded ? FolderOpenIcon : FolderIcon;
    nodeIconElement = (
      <FolderSpecificIcon
        sx={{
          fontSize: '1.1rem',
          color: itemStyle.iconColor || itemStyle.textColor,
        }}
      />
    );
  } else if (ItemIconComponent) {
    // Leaf node with a status-derived icon
    nodeIconElement = (
      <ItemIconComponent
        sx={{
          fontSize: '1.1rem',
          color: itemStyle.iconColor || itemStyle.textColor,
        }}
      />
    );
  } else {
    // Fallback for other leaf types or if no icon from style
    nodeIconElement = (
      <DescriptionIcon
        sx={{
          fontSize: '1.1rem',
          color: 'var(--gm3-color-on-surface-variant)',
        }}
      />
    );
  }

  const handleItemClick = () => {
    if (hasChildren && !node.isLeaf && !node.isClickable) {
      // Folder node click
      toggleNodeExpansion(node.id);
    } else if (
      node.isClickable &&
      node.testId &&
      node.variantHash &&
      onSelectTestVariant
    ) {
      // Clickable leaf node
      onSelectTestVariant(node.testId, node.variantHash);
      if (closeDrawer) {
        closeDrawer();
      }
    }
    // Non-clickable leaves (like failure reasons) do nothing on click.
  };

  return (
    <React.Fragment key={node.id}>
      <ListItemButton
        selected={
          currentTestId === node.testId &&
          currentVariantHash === node.variantHash &&
          node.isClickable
        }
        onClick={handleItemClick}
        sx={{ pl: node.level * 2 + 1, py: 0.35, pr: 1 }}
      >
        <ListItemIcon
          sx={{
            minWidth: 'auto',
            mr: 0.5,
            display: 'flex',
            alignItems: 'center',
          }}
        >
          {hasChildren && !node.isLeaf ? ( // Show expander only for non-leaf folder nodes
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                toggleNodeExpansion(node.id);
              }}
              sx={{ p: 0.25 }}
              aria-label={isExpanded ? 'Collapse' : 'Expand'}
            >
              {isExpanded ? (
                <ExpandMoreIcon fontSize="inherit" />
              ) : (
                <ChevronRightIcon fontSize="inherit" />
              )}
            </IconButton>
          ) : (
            <Box sx={{ width: 24, height: 24 }} /> // Spacer for alignment
          )}
        </ListItemIcon>
        <ListItemIcon
          sx={{
            minWidth: 'auto',
            mr: 0.75,
            display: 'flex',
            alignItems: 'center',
          }}
        >
          {nodeIconElement}
        </ListItemIcon>
        <ListItemText
          primary={node.label}
          secondary={
            !node.isLeaf &&
            hasChildren &&
            (node.failedTests !== undefined || node.totalTests !== undefined)
              ? `${node.failedTests || 0} failed (${node.totalTests || 0} total)`
              : undefined
          }
          primaryTypographyProps={{
            variant: 'body2',
            sx: {
              fontWeight:
                currentTestId === node.testId &&
                currentVariantHash === node.variantHash &&
                node.isClickable
                  ? 'bold'
                  : 'normal',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              // Apply text color from style primarily for clickable items, others use default
              color:
                node.isClickable || (!node.isLeaf && hasChildren)
                  ? itemStyle.textColor
                  : 'text.primary',
            },
          }}
          secondaryTypographyProps={{
            variant: 'caption',
            sx: {
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            },
          }}
        />
        {node.tag && node.tagColor && (
          <Chip
            label={node.tag}
            size="small"
            sx={{
              ml: 1,
              height: '18px',
              fontSize: '0.7rem',
              borderRadius: '4px',
              backgroundColor: getStatusStyle(node.tagColor).backgroundColor,
              color: getStatusStyle(node.tagColor).textColor,
              borderColor: getStatusStyle(node.tagColor).borderColor,
              borderStyle: getStatusStyle(node.tagColor).borderColor
                ? 'solid'
                : 'none',
              borderWidth: getStatusStyle(node.tagColor).borderColor
                ? '1px'
                : '0',
            }}
          />
        )}
      </ListItemButton>
      {hasChildren &&
        !node.isLeaf && ( // Only render children block for expandable non-leaf nodes
          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
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
                  closeDrawer={closeDrawer}
                />
              ))}
            </List>
          </Collapse>
        )}
    </React.Fragment>
  );
}
