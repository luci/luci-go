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

import { Tooltip, Link } from '@mui/material';
import { grey } from '@mui/material/colors';

import { TreeNodeColors, TreeNodeLabels, ObjectNode } from '../types';

/**
 * Props for the node text.
 */
interface NodeTextProps {
  node: ObjectNode;
  colors?: TreeNodeColors;
}

function NodeText({ node, colors }: NodeTextProps) {
  const nodeInnerText = decodeURIComponent(node.name);

  if (node.size === 0) {
    return (
      <span
        css={{
          color: colors?.unsupportedColor ?? grey[600],
        }}
      >
        {nodeInnerText}
      </span>
    );
  }

  return <Link>{nodeInnerText}</Link>;
}

/**
 * Props for the node text.
 */
interface LeafNodeTextProps {
  node: ObjectNode;
  hasLeafNodeClick: boolean;
  colors?: TreeNodeColors;
  labels: TreeNodeLabels;
}

/**
 * Represents the leaf node text in the logs tree.
 */
export function LeafNodeText({
  node,
  hasLeafNodeClick,
  colors,
  labels,
}: LeafNodeTextProps) {
  if (hasLeafNodeClick) {
    return node.viewingsupported ? (
      <>{decodeURIComponent(node.name)}</>
    ) : (
      <NodeText node={node} colors={colors} />
    );
  } else {
    return (
      <Tooltip title={labels.nonSupportedLeafNodeTooltip} placement="bottom">
        <NodeText node={node} colors={colors} />
      </Tooltip>
    );
  }
}
