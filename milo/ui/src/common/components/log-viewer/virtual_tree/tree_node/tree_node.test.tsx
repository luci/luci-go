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

import '@testing-library/jest-dom';

import { render, fireEvent, screen } from '@testing-library/react';

import { TreeData, TreeNodeData } from '../types';

import { TreeNode } from './tree_node';

const simpleLeafNode: TreeNodeData = {
  id: '1',
  name: 'leaf-node',
  children: [],
};

const simpleNode: TreeNodeData = {
  id: '2',
  name: 'node',
  children: [simpleLeafNode],
};

const LEAF_TREE_NODE_DATA: TreeData<TreeNodeData> = {
  id: '1',
  name: 'leaf-node',
  children: [],
  level: 2,
  data: simpleLeafNode,
  isLeafNode: true,
  isOpen: true,
  parent: undefined,
};

const TREE_NODE_DATA: TreeData<TreeNodeData> = {
  id: '2',
  name: 'node',
  children: [LEAF_TREE_NODE_DATA],
  level: 1,
  data: simpleNode,
  isLeafNode: false,
  isOpen: true,
  parent: undefined,
};

describe('<TreeNode />', () => {
  const mockNodeSelectFn = jest.fn();
  const mockNodeToggleFn = jest.fn();
  it('should render the leaf node', () => {
    render(
      <TreeNode
        data={LEAF_TREE_NODE_DATA}
        onNodeSelect={mockNodeSelectFn}
        onNodeToggle={mockNodeToggleFn}
      />,
    );
    const node = screen.getByTestId('default-leaf-node-1');
    fireEvent.click(node);

    expect(mockNodeSelectFn).toHaveBeenCalled();
    expect(screen.queryByTestId('default-tree-node-1')).toBeInTheDocument();
    expect(screen.queryByTestId('default-leaf-node-1')).toBeInTheDocument();
    expect(screen.queryByTestId('default-node-1')).not.toBeInTheDocument();
  });

  it('should render node', () => {
    render(
      <TreeNode
        data={TREE_NODE_DATA}
        onNodeSelect={mockNodeSelectFn}
        onNodeToggle={mockNodeToggleFn}
      />,
    );
    const node = screen.getByTestId('default-tree-node-2');
    const nodeCollapseIcon = screen.getByTestId('ExpandMoreIcon');
    fireEvent.click(node);
    fireEvent.click(nodeCollapseIcon);

    expect(mockNodeSelectFn).toHaveBeenCalled();
    expect(mockNodeToggleFn).toHaveBeenCalled();
    expect(screen.queryByTestId('default-tree-node-2')).toBeInTheDocument();
    expect(screen.queryByTestId('default-leaf-node-2')).not.toBeInTheDocument();
    expect(screen.queryByTestId('default-node-2')).toBeInTheDocument();
  });
});
