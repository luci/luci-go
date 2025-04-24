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

import { lightBlue } from '@mui/material/colors';
import { ReactNode, useRef, useState } from 'react';

import { IndentBorder } from '../indent_border';
import {
  TreeData,
  TreeNodeData,
  ObjectNode,
  TreeFontVariant,
  TreeNodeColors,
  TreeNodeLabels,
} from '../types';

import { TreeInternalNode } from './tree_internal_node';
import { TreeLeafNode } from './tree_leaf_node';

/**
 * Props for the Tree node.
 */
export interface LogsTreeNodeProps<T extends TreeNodeData> {
  treeNodeData: TreeData<T>;
  index: number;
  collapseIcon?: ReactNode;
  expandIcon?: ReactNode;
  treeFontSize?: TreeFontVariant;
  iconFontSize?: string;
  inlineActions?: ReactNode;
  treeIndentBorder?: boolean;
  treeNodeIndentation: number;
  isSelected?: boolean;
  isSearchMatch?: boolean;
  isActiveSelection?: boolean;
  isSpecialNode?: boolean;
  labels: TreeNodeLabels;
  colors?: TreeNodeColors;
  onNodeToggle: (treeNodeData: TreeData<T>) => void;
  onNodeSelect: (treeNodeData: TreeData<T>) => void;
  logActivityTrigger?: (path: string) => void;
  onLeafNodeClick?: (newlySelectedNode: T, mergeFile?: boolean) => void;
  onUnsupportedLeafNodeClick: (node: T) => void;
}

/** Logs tree node representing a file/dir in the directory tree.  */
export function LogsTreeNode({
  treeNodeData,
  index,
  collapseIcon,
  treeFontSize,
  iconFontSize,
  expandIcon,
  inlineActions,
  treeIndentBorder,
  treeNodeIndentation,
  isSelected,
  isSearchMatch,
  isActiveSelection,
  isSpecialNode,
  labels,
  colors,
  onNodeToggle,
  onNodeSelect,
  logActivityTrigger,
  onLeafNodeClick,
  onUnsupportedLeafNodeClick,
}: LogsTreeNodeProps<ObjectNode>) {
  // Reference to the node
  const nodeRef = useRef<HTMLDivElement>(null);

  // TreeData for the node.
  const [isHovered, setIsHovered] = useState(false);
  const [isInlineActionsHovered, setIsInlineActionsHovered] =
    useState<boolean>(false);

  const handleNodeOnClick = (node: ObjectNode) => {
    // Disable the click when the user is hovering over the inline actions.
    if (isInlineActionsHovered) return;

    logActivityTrigger?.(node.deeplinkpath ?? '');
    onNodeSelect(treeNodeData);

    if (onLeafNodeClick && node.viewingsupported) {
      onLeafNodeClick?.(node);
    } else {
      onUnsupportedLeafNodeClick?.(node);
    }
  };

  const backgroundStyles = {
    background: colors?.defaultBackgroundColor ?? lightBlue[50],
    borderRadius: '10px',
  };

  // Adds background color for selected nodes.
  const selectedNodeStyle = isSelected ? backgroundStyles : {};

  // On hover highlights the node.
  const highlightOnHover = isHovered ? backgroundStyles : {};

  return (
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events
    <div
      role="button"
      tabIndex={0}
      ref={nodeRef}
      onClick={() => handleNodeOnClick(treeNodeData.data)}
      data-testid={`node-${treeNodeData.name}`}
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      css={{
        // flex -> required for border indentations.
        // inline-table -> recognizes and adds background color for
        // the extra white space added by browser after rendering text.
        display: `${treeIndentBorder ? 'flex' : 'inline-table'}`,
        flexWrap: 'wrap',
        alignContent: 'center',
        paddingLeft: `${
          treeIndentBorder ? 0 : treeNodeData.level * treeNodeIndentation!
        }px`,
        width: '100%',
        boxSizing: 'border-box',
        cursor: treeNodeData.data.size === 0 ? 'default' : 'pointer',
        ...selectedNodeStyle,
        ...highlightOnHover,
      }}
    >
      {/* Renders border lines from parent to child */}
      {treeIndentBorder ? (
        <IndentBorder
          index={index}
          level={treeNodeData.level}
          nodeIndentation={treeNodeIndentation}
        />
      ) : (
        <></>
      )}
      {/* Leaf nodes are files. */}
      {treeNodeData.isLeafNode ? (
        <TreeLeafNode
          treeNodeData={treeNodeData}
          treeFontSize={treeFontSize}
          iconFontSize={iconFontSize}
          isSearchMatch={isSearchMatch}
          isActiveSelection={isActiveSelection}
          isHovered={isHovered}
          isSpecialNode={isSpecialNode}
          inlineActions={inlineActions}
          colors={colors}
          labels={labels}
          onLeafNodeClick={onLeafNodeClick}
          onMouseEnter={() => setIsInlineActionsHovered(true)}
          onMouseLeave={() => setIsInlineActionsHovered(false)}
        />
      ) : (
        <TreeInternalNode
          treeNodeData={treeNodeData}
          collapseIcon={collapseIcon}
          expandIcon={expandIcon}
          treeFontSize={treeFontSize}
          isActiveSelection={isActiveSelection}
          isSearchMatch={isSearchMatch}
          colors={colors}
          onNodeSelect={onNodeSelect}
          onNodeToggle={onNodeToggle}
        />
      )}
    </div>
  );
}
