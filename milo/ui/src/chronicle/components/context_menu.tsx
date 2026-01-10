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

import { ChronicleNode, GroupMode } from '../utils/graph_builder';

export interface ContextMenuState {
  mouseX: number;
  mouseY: number;
  node: ChronicleNode;
  // The group ID this node belongs to (for expanding itself).
  selfGroupId?: number;
  // Child group IDs (for collapsing children).
  childGroupIds?: number[];
}

export interface ContextMenuProps {
  contextMenuState: ContextMenuState | undefined;
  onClose: () => void;
  onSetGroupMode: (
    groupIds: number[],
    mode: GroupMode,
    anchorNodeId?: string,
  ) => void;
}

export function ContextMenu({
  contextMenuState,
  onClose,
  onSetGroupMode,
}: ContextMenuProps) {
  const isOpen = !!contextMenuState;

  const handleExpandSelf = () => {
    if (contextMenuState?.selfGroupId) {
      onSetGroupMode([contextMenuState.selfGroupId], GroupMode.EXPANDED);
    }
    onClose();
  };

  const childGroupIds = contextMenuState?.childGroupIds || [];
  const hasChildren = childGroupIds.length > 0;

  const handleCollapseAllChildren = () => {
    onSetGroupMode(
      childGroupIds,
      GroupMode.COLLAPSE_ALL,
      contextMenuState?.node.id,
    );
    onClose();
  };

  const handleCollapseSuccessfulChildren = () => {
    onSetGroupMode(
      childGroupIds,
      GroupMode.COLLAPSE_SUCCESS_ONLY,
      contextMenuState?.node.id,
    );
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
