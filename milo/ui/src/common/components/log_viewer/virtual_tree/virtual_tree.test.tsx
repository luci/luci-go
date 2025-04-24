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

import AddBoxIconOutlined from '@mui/icons-material/AddBoxOutlined';
import IndeterminateCheckBoxOutlined from '@mui/icons-material/IndeterminateCheckBoxOutlined';
import { render, fireEvent, screen } from '@testing-library/react';

import {
  ACTIVE_NODE_SELECTION_BACKGROUND_COLOR,
  SEARCH_MATCHED_BACKGROUND_COLOR,
} from './tree_node';
import { TreeNodeData } from './types';
import { VirtualTree } from './virtual_tree';

// TODO(b/279922669): Move common constants to a utility file
// Returns nth parent of the element if it exists or returns null.
const getNthParent = (element: HTMLElement, n: number): HTMLElement | null => {
  let nthParent: HTMLElement | null = element;
  while (n !== 0 && nthParent !== null) {
    nthParent = nthParent.parentElement;
    n--;
  }
  return nthParent;
};

const treeData: TreeNodeData[] = [
  {
    id: 1,
    name: 'root1',
    children: [
      {
        id: 3,
        name: 'dir1',
        children: [
          {
            id: 7,
            name: 'leafNode1',
            children: [],
          },
          {
            id: 8,
            name: 'leafNode2',
            children: [],
          },
        ],
      },
      {
        id: 4,
        name: 'dir2',
        children: [
          {
            id: 9,
            name: 'leafNode3',
            children: [],
          },
        ],
      },
    ],
  },
  {
    id: 2,
    name: 'root2',
    children: [
      {
        id: 5,
        name: 'dir3',
        children: [
          {
            id: 10,
            name: 'leafNode4',
            children: [],
          },
          {
            id: 11,
            name: 'leafNode5',
            children: [],
          },
        ],
      },
      {
        id: 6,
        name: 'dir4',
        children: [
          {
            id: 12,
            name: 'leafNode6',
            children: [],
          },
        ],
      },
    ],
  },
];

describe('<VirtualTree />', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  describe('when using default nodes', () => {
    const nodeSelectMockFn = jest.fn();
    const nodeToggleMockFn = jest.fn();

    beforeEach(() => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          onNodeSelect={nodeSelectMockFn}
          onNodeToggle={nodeToggleMockFn}
          isTreeCollapsed={false}
        />,
      );
    });

    it('should render in tree structure with proper indentation', () => {
      // Default node indentation
      const defaultIndentation = 32;
      // root nodes are at level 0 in the tree structure
      const rootNodeLevel = 0;
      // leaf nodes are at level 2 in the tree structure
      const leafNodeLevel = 2;
      // intermediate nodes are at level 1 in the tree structure
      const intermediateNodeLevel = 1;
      const rootNodes = screen.queryAllByText('root', { exact: false });
      const leafNodes = screen.queryAllByTestId('default-leaf-node-', {
        exact: false,
      });
      const intermediateNodes = screen.queryAllByText('dir', {
        exact: false,
      });

      expect(rootNodes).toHaveLength(2);
      expect(leafNodes).toHaveLength(6);
      expect(intermediateNodes).toHaveLength(4);

      rootNodes.forEach((node) => {
        expect(node.parentElement).toHaveStyle({
          'margin-left': `${rootNodeLevel * defaultIndentation}px`,
        });
      });

      leafNodes.forEach((node) => {
        const parent = getNthParent(node, /* nth parent */ 3);
        if (!parent) {
          return;
        }
        expect(parent).toHaveStyle({
          'margin-left': `${leafNodeLevel * defaultIndentation}px`,
        });
      });

      intermediateNodes.forEach((node) => {
        const parent = getNthParent(node, /* nth parent */ 4);
        if (parent) {
          return;
        }
        expect(parent).toHaveStyle({
          'margin-left': `${intermediateNodeLevel * defaultIndentation}px`,
        });
      });
    });

    it('should render the default tree with data', () => {
      expect(screen.getByTestId('virtual-tree')).toBeInTheDocument();
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(6);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(6);
      expect(screen.queryAllByTestId('ExpandMoreIcon')).toHaveLength(6);
    });

    it('should toggle expand and collapse nodes', () => {
      // Collapse root1
      const root1NodeExpand = screen.getAllByTestId('ExpandMoreIcon').at(0);
      fireEvent.click(root1NodeExpand!);
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(3);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(4);

      // Expand root1
      const root1NodeCollapse = screen.getAllByTestId('ChevronRightIcon').at(0);
      fireEvent.click(root1NodeCollapse!);
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(6);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(6);

      // Collapse and Expand for root1 are 2 toggle actions.
      expect(nodeToggleMockFn).toHaveBeenCalledTimes(2);
    });

    it('should toggle expand and collapse nodes without affecting underlying toggled state of children', () => {
      // Collapse dir 2
      const dir2NodeExpand = screen.getAllByTestId('ExpandMoreIcon').at(2);
      fireEvent.click(dir2NodeExpand!);
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(5);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(6);

      // Collapse root1
      const root1NodeExpand = screen.getAllByTestId('ExpandMoreIcon').at(0);
      fireEvent.click(root1NodeExpand!);
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(3);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(4);

      // Expand root1 with collapsed dir2
      const root1NodeCollapse = screen.getAllByTestId('ChevronRightIcon').at(0);
      fireEvent.click(root1NodeCollapse!);
      expect(
        screen.queryAllByTestId('default-leaf-node-', { exact: false }),
      ).toHaveLength(5);
      expect(
        screen.queryAllByTestId('default-node-', { exact: false }),
      ).toHaveLength(6);

      // Collapse and Expand for root1 with collapse dir2 are
      // 3 toggle actions.
      expect(nodeToggleMockFn).toHaveBeenCalledTimes(3);
    });
  });

  describe('when rendering collapsed tree', () => {
    it('should collapse and expand the whole tree', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          isTreeCollapsed={true}
        />,
      );

      const rootNodes = screen.queryAllByText('root', { exact: false });
      const leafNodes = screen.queryAllByTestId('default-leaf-node-', {
        exact: false,
      });
      const intermediateNodes = screen.queryAllByText('dir', {
        exact: false,
      });

      // Collapsed tree only displays the roots.
      expect(rootNodes).toHaveLength(2);
      // Collapsed tree does not display any nodes except for root nodes.
      expect(leafNodes).toHaveLength(0);
      expect(intermediateNodes).toHaveLength(0);
    });
  });

  describe('when using custom icons for default nodes', () => {
    it('should render custom icons', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          collapseIcon={<IndeterminateCheckBoxOutlined />}
          expandIcon={<AddBoxIconOutlined />}
        />,
      );

      expect(
        screen.queryAllByTestId('IndeterminateCheckBoxOutlinedIcon'),
      ).toHaveLength(6);
      expect(screen.queryAllByTestId('AddBoxOutlinedIcon')).toHaveLength(0);

      const root1NodeExpand = screen
        .getAllByTestId('IndeterminateCheckBoxOutlinedIcon')
        .at(0);
      fireEvent.click(root1NodeExpand!);
      expect(
        screen.queryAllByTestId('IndeterminateCheckBoxOutlinedIcon'),
      ).toHaveLength(3);
      expect(screen.queryAllByTestId('AddBoxOutlinedIcon')).toHaveLength(1);
    });
  });

  describe('when using search with default nodes', () => {
    let pattern = 'dir';
    const onSearchMatchFoundMockFn = jest.fn();
    const defaultActiveIndex = 0;

    it('should highlight search matches', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          searchOptions={{ pattern: 'dir' }}
          onSearchMatchFound={onSearchMatchFoundMockFn}
        />,
      );

      const searchedNodes = screen.queryAllByText('dir', { exact: false });

      expect(searchedNodes).toHaveLength(4);
      expect(onSearchMatchFoundMockFn).toHaveBeenCalledWith(
        defaultActiveIndex,
        /* totalMatches= */ 4,
      );

      // Expect to have background color of yellow[400]
      searchedNodes.forEach((matchedNode) => {
        expect(matchedNode.parentElement).toHaveStyle({
          'background-color': `${SEARCH_MATCHED_BACKGROUND_COLOR}`,
        });
      });
    });

    it('should highlight nested search matches', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          searchOptions={{ pattern: 'root1/dir1' }}
          onSearchMatchFound={onSearchMatchFoundMockFn}
        />,
      );
      const totalMatches = 1;

      const searchedNodes = screen.queryAllByText('dir1', { exact: false });

      expect(searchedNodes).toHaveLength(totalMatches);
      expect(onSearchMatchFoundMockFn).toHaveBeenCalledWith(
        defaultActiveIndex,
        totalMatches,
      );
      expect(searchedNodes[0].parentElement).toHaveStyle({
        'background-color': `${SEARCH_MATCHED_BACKGROUND_COLOR}`,
      });
    });

    it('should bypass tree search with invalid pattern', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          searchOptions={{ pattern: '*/dir1' }}
          onSearchMatchFound={onSearchMatchFoundMockFn}
        />,
      );

      expect(onSearchMatchFoundMockFn).toHaveBeenCalledWith(-1, 0);
    });

    it('should highlight search matches with active index', () => {
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          searchOptions={{ pattern }}
          onSearchMatchFound={onSearchMatchFoundMockFn}
          searchActiveIndex={0}
        />,
      );

      const searchedNodes = screen.queryAllByText('dir', { exact: false });

      expect(searchedNodes).toHaveLength(4);
      expect(onSearchMatchFoundMockFn).toHaveBeenCalledWith(
        defaultActiveIndex,
        /* totalMatches= */ 4,
      );

      // The first index has deeporange[300] due to active index and
      // others have yellow[400].
      searchedNodes.forEach((matchedNode, index) => {
        expect(matchedNode.parentElement).toHaveStyle({
          'background-color': `${
            index === 0
              ? ACTIVE_NODE_SELECTION_BACKGROUND_COLOR
              : SEARCH_MATCHED_BACKGROUND_COLOR
          }`,
        });
      });

      pattern = '';
      // Rerender with updated pattern to trigger the useEffect in component.
      render(
        <VirtualTree
          disableVirtualization
          root={treeData}
          searchOptions={{ pattern }}
        />,
      );
    });
  });
});
