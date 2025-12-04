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
import { Node } from 'reactflow';

export interface ContextMenuState {
  mouseX: number;
  mouseY: number;
  node: Node;
}

export interface ContextMenuProps {
  contextMenuState: ContextMenuState | undefined;
  onClose: () => void;
  onCollapseSimilar: (parentHash: number) => void;
  onExpandGroup: (parentHash: number) => void;
}

export function ContextMenu({
  contextMenuState,
  onClose,
  onCollapseSimilar,
  onExpandGroup,
}: ContextMenuProps) {
  const isOpen = !!contextMenuState;

  const handleCollapse = () => {
    if (contextMenuState?.node.data?.parentHash) {
      onCollapseSimilar(contextMenuState.node.data.parentHash);
    }
    onClose();
  };

  const handleExpand = () => {
    if (contextMenuState?.node.data?.parentHash) {
      onExpandGroup(contextMenuState.node.data.parentHash);
    }
    onClose();
  };

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
      {contextMenuState?.node.data.isCollapsed ? (
        <MenuItem onClick={handleExpand}>Expand successful nodes</MenuItem>
      ) : (
        <MenuItem onClick={handleCollapse}>Collapse successful nodes</MenuItem>
      )}
    </Menu>
  );
}
