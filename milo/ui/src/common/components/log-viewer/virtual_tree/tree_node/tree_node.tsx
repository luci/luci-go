// Copyright 2024 The LUCI Authors.
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

/* eslint-disable jsx-a11y/no-static-element-interactions */
/* eslint-disable jsx-a11y/click-events-have-key-events */

import {
  ChevronRight as ChevronRightIcon,
  ExpandMore as ExpandMoreIcon,
} from '@mui/icons-material';
import { Stack, Typography } from '@mui/material';
import React, { useEffect, useState } from 'react';

import { TreeData, TreeNodeData } from '../types';

import {
  ACTIVE_NODE_SELECTION_BACKGROUND_COLOR,
  SEARCH_MATCHED_BACKGROUND_COLOR,
  SELECTED_NODE_BACKGROUND_COLOR,
} from './constants';

/**
 * Props for default node renderer.
 */
export interface TreeNodeProps<T extends TreeNodeData> {
  data: TreeData<T>;
  collapseIcon?: React.ReactNode;
  expandIcon?: React.ReactNode;
  isSelected?: boolean;
  isSearchMatch?: boolean;
  isActiveSelection?: boolean;
  onNodeSelect?: (treeNodeData: TreeData<T>) => void;
  onNodeToggle?: (treeNodeData: TreeData<T>) => void;
}

/**
 * Returns default tree node component with basic features of
 * displaying the data and node toggle and node select props.
 */
export function TreeNode<T extends TreeNodeData>({
  data,
  collapseIcon,
  expandIcon,
  isSelected,
  isSearchMatch,
  isActiveSelection,
  onNodeSelect,
  onNodeToggle,
}: TreeNodeProps<T>) {
  const [treeNodeData, setTreeNodeData] = useState<TreeData<T>>(data);

  // Gets background to the node based on the treeData attribute.
  const getNodeBackgroundColor = () => {
    if (isActiveSelection) return ACTIVE_NODE_SELECTION_BACKGROUND_COLOR;
    if (isSearchMatch) return SEARCH_MATCHED_BACKGROUND_COLOR;
    if (isSelected && treeNodeData.isLeafNode)
      return SELECTED_NODE_BACKGROUND_COLOR;
    return undefined;
  };

  useEffect(() => {
    setTreeNodeData(data);
  }, [data]);

  return (
    <div
      data-testid={`default-tree-node-${data.id}`}
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignContent: 'center',
      }}
    >
      {treeNodeData.isLeafNode ? (
        <Typography
          component="span"
          sx={{ backgroundColor: getNodeBackgroundColor() }}
        >
          <div
            data-testid={`default-leaf-node-${data.id}`}
            style={{ cursor: 'pointer' }}
            onClick={() => onNodeSelect?.(treeNodeData)}
          >
            {treeNodeData.name}
          </div>
        </Typography>
      ) : (
        // indicates folder/directory to render the expand and collapse icons.
        <Stack
          spacing={0.5}
          direction={'row'}
          style={{ display: 'flex', alignItems: 'center' }}
        >
          <div
            data-testid={`default-node-${data.id}`}
            onClick={() => onNodeToggle?.(treeNodeData)}
            style={{ display: 'flex', alignItems: 'center' }}
          >
            {treeNodeData.isOpen
              ? (collapseIcon ?? <ExpandMoreIcon sx={{ fontSize: '18px' }} />)
              : (expandIcon ?? <ChevronRightIcon sx={{ fontSize: '18px' }} />)}
          </div>
          <Typography
            component="span"
            fontWeight="bold"
            data-testid={`name-${treeNodeData.data.name}`}
            sx={{
              display: 'flex',
              backgroundColor: getNodeBackgroundColor(),
            }}
          >
            <div
              style={{ cursor: 'pointer' }}
              onClick={() => onNodeSelect?.(treeNodeData)}
            >
              {treeNodeData.data.name}
            </div>
          </Typography>
        </Stack>
      )}
    </div>
  );
}
