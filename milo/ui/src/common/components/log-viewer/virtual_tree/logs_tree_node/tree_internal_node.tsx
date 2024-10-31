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

/* eslint-disable jsx-a11y/click-events-have-key-events */

import { Stack, Typography } from '@mui/material';
import { ReactNode } from 'react';

import {
  TreeData,
  ObjectNode,
  TreeFontVariant,
  TreeNodeColors,
} from '../types';
import { getNodeBackgroundColor } from '../utils';

/**
 * Props for the Tree node.
 */
interface TreeInternalNodeProps {
  treeNodeData: TreeData<ObjectNode>;
  collapseIcon?: ReactNode;
  expandIcon?: ReactNode;
  treeFontSize?: TreeFontVariant;
  isSearchMatch?: boolean;
  isActiveSelection?: boolean;
  colors?: TreeNodeColors;
  onNodeToggle: (treeNodeData: TreeData<ObjectNode>) => void;
  onNodeSelect: (treeNodeData: TreeData<ObjectNode>) => void;
}

export function TreeInternalNode({
  treeNodeData,
  collapseIcon,
  expandIcon,
  treeFontSize,
  isSearchMatch,
  isActiveSelection,
  colors,
  onNodeSelect,
  onNodeToggle,
}: TreeInternalNodeProps) {
  return (
    <Stack
      spacing={0.5}
      direction={'row'}
      sx={{ display: 'flex', alignItems: 'center' }}
    >
      <span
        role="button"
        tabIndex={0}
        onClick={() => onNodeToggle(treeNodeData)}
        css={{ display: 'flex', alignItems: 'center' }}
      >
        {treeNodeData.isOpen ? collapseIcon : expandIcon}
      </span>
      <Typography
        component="span"
        fontWeight="bold"
        data-testid={`name-${treeNodeData.name}`}
        variant={treeFontSize ?? /* default value */ 'body1'}
      >
        <span
          role="button"
          tabIndex={0}
          onClick={() => onNodeSelect(treeNodeData)}
          css={{
            backgroundColor: getNodeBackgroundColor(
              colors,
              treeNodeData.data.deeplinked,
              isActiveSelection,
              isSearchMatch,
            ),
          }}
        >
          {decodeURIComponent(treeNodeData.name)}
        </span>
      </Typography>
    </Stack>
  );
}
