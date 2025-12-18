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

import { Menu, MenuItem } from '@mui/material';

import { CheckResultStatus } from '../utils/check_utils';
import { ChronicleNode } from '../utils/graph_builder';

/**
 * Information about a collapsible group of children for the current node
 * (on which the context menu is being shown).
 */
export interface CollapsibleChildGroup {
  hash: number;
  status: CheckResultStatus;
}

export interface ContextMenuState {
  mouseX: number;
  mouseY: number;
  node: ChronicleNode;
  collapsibleGroups?: CollapsibleChildGroup[];
}

export interface ContextMenuProps {
  contextMenuState: ContextMenuState | undefined;
  onClose: () => void;
  onCollapse: (hashes: number[], focusNodeId?: string) => void;
  onExpand: (hashes: number[]) => void;
}

export function ContextMenu({
  contextMenuState,
  onClose,
  onCollapse,
  onExpand,
}: ContextMenuProps) {
  const isOpen = !!contextMenuState;

  const handleExpandSelf = () => {
    if (contextMenuState?.node.data?.dependencyHash) {
      onExpand([contextMenuState.node.data.dependencyHash]);
    }
    onClose();
  };

  const childGroups = contextMenuState?.collapsibleGroups || [];
  const hasChildren = childGroups.length > 0;

  const handleCollapseAllChildren = () => {
    const hashes = childGroups.map((g) => g.hash);
    onCollapse(hashes, contextMenuState?.node.id);
    onClose();
  };

  const handleCollapseSuccessfulChildren = () => {
    const hashes = childGroups
      .filter((g) => g.status === CheckResultStatus.SUCCESS)
      .map((g) => g.hash);
    onCollapse(hashes, contextMenuState?.node.id);
    onClose();
  };

  const isSelfCollapsed = !!contextMenuState?.node.data?.isCollapsed;

  return (
    <Menu
      open={isOpen}
      onClose={onClose}
      anchorReference="anchorPosition"
      anchorPosition={
        contextMenuState
          ? { top: contextMenuState.mouseY, left: contextMenuState.mouseX }
          : undefined
      }
    >
      {/* Expand action if I am a collapsed node. */}
      {isSelfCollapsed && (
        <MenuItem onClick={handleExpandSelf}>Expand</MenuItem>
      )}

      {/* Collapse actions if I am a parent with collapsible children */}
      {hasChildren && [
        <MenuItem key="collapse-all" onClick={handleCollapseAllChildren}>
          Collapse all children
        </MenuItem>,
        <MenuItem
          key="collapse-success"
          onClick={handleCollapseSuccessfulChildren}
        >
          Collapse successful children
        </MenuItem>,
      ]}
    </Menu>
  );
}
