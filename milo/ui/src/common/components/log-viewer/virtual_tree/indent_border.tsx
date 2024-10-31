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

import { ReactElement, useMemo } from 'react';

/**
 * Props for the Indent border.
 */
export interface IndentBorderProps {
  // Indicates the depth of the node in the tree
  level: number;
  // Index of the node in the flattened tree.
  index: number;
  // Indicates the indentation of the child node from the parent.
  nodeIndentation: number;
}

/**
 * Renders the indent border for the Virtual tree connecting the leaf nodes to
 * their parent nodes.
 */
export function IndentBorder({
  level,
  index,
  nodeIndentation,
}: IndentBorderProps) {
  const indentBorder = useMemo(() => {
    const indentDivs = new Array<ReactElement>();
    for (let i = 0; i < level; i++) {
      indentDivs.push(
        <div
          style={{
            height: '100%',
            marginLeft: `${i === 0 ? 12 : nodeIndentation}px`,
            marginRight: `${i === level - 1 ? '8px' : '0'}`,
            borderLeft: '1px solid rgba(0,0,0,0.4)',
          }}
          key={i}
        ></div>,
      );
    }
    return indentDivs;
  }, [level, nodeIndentation]);

  return (
    <div
      style={{
        display: 'flex',
        paddingRight: `${
          index === 0
            ? 4
            : // eslint-disable-next-line max-len
              /* adjust the starting element to line under the parent border */ nodeIndentation /
              2
        }px`,
      }}
    >
      {level > 0 && indentBorder}
    </div>
  );
}
