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

import { deepOrange, teal, yellow } from '@mui/material/colors';

import {
  SearchTreeMatch,
  TreeData,
  TreeNodeColors,
  TreeNodeData,
} from './types';

// Copy from the readonly data source to convert the id field to string
// from number.
function copyTree<T extends TreeNodeData>(tree: readonly T[]): T[] {
  const rootCopy = new Array<T>();
  for (const node of tree) {
    rootCopy.push(copyTreeHelper(node));
  }
  return rootCopy;
}

// Helper function that recursively copies the tree.
function copyTreeHelper<T extends TreeNodeData>(node: T): T {
  const clonedRoot = { ...node, id: node.id.toString() };
  if (!node.children) return clonedRoot;

  const childArray = new Array<T>();
  for (const child of node.children) {
    childArray.push(copyTreeHelper(child as T));
  }
  clonedRoot.children = childArray;
  return clonedRoot;
}

/**
 * Generates the Tree Data List item by traversing root effectively flattening
 * the tree in pre order traversal.
 * @param root - source of the tree.
 * @param treeDataList - list in which the tree data will be stored.
 * @param level - indicates the depth of a node in the tree.
 * @param parent - links to the parent of the node or is undefined for roots.
 * @param setActiveSelectionFn - helper func to set the property of the node
 *                               indicating whether it has been selected..
 * @returns List of flattened tree data from source.
 */
export function generateTreeDataList<T extends TreeNodeData>(
  root: readonly T[],
  treeDataList: Array<TreeData<T>>,
  level: number,
  parent?: TreeData<T>,
): Array<TreeData<T>> {
  const copiedTree = copyTree(root);
  const createdTreeDataList = createTreeData(copiedTree);
  const idToTreeData = new Map<string, TreeData<T>>(
    createdTreeDataList.map((treeData) => [treeData.id, treeData]),
  );
  return generateTreeDataListHelper(
    copiedTree,
    treeDataList,
    level,
    parent,
    idToTreeData,
  );
}

function createTreeData<T extends TreeNodeData>(tree: T[]) {
  return createTreeDataHelper(tree, [], 0);
}

function isLeafNode<T extends TreeNodeData>(node: T, level: number) {
  // Mark as a leaf node if the node doesn't have any children and it is not
  // the root node.
  return node.children.length === 0 && level !== 0;
}

function createTreeDataHelper<T extends TreeNodeData>(
  tree: T[],
  treeDataList: TreeData<T>[],
  level: number,
) {
  for (const node of tree) {
    treeDataList.push({
      id: node.id as string,
      name: node.name,
      data: node as T,
      children: new Array<TreeData<T>>(),
      level,

      isLeafNode: isLeafNode(node, level),
      isOpen: true,
      parent: undefined,
    });

    if (node.children) {
      treeDataList = createTreeDataHelper(
        node.children as T[],
        treeDataList,
        level + 1,
      );
    }
  }
  return treeDataList;
}

// Helper function to generate the tree data list.
function generateTreeDataListHelper<T extends TreeNodeData>(
  root: readonly T[],
  treeDataList: Array<TreeData<T>>,
  level: number,
  parent: TreeData<T> | undefined,
  idToTreeData: Map<string, TreeData<T>>,
): Array<TreeData<T>> {
  for (const node of root) {
    const treeData = getTreeData(
      node,
      level,
      parent,
      idToTreeData,
    ) as TreeData<T>;
    treeDataList.push(treeData);
    if (node.children) {
      treeDataList = generateTreeDataListHelper(
        node.children as T[],
        treeDataList,
        level + 1,
        treeData,
        idToTreeData,
      );
    }
  }
  return treeDataList;
}

/**
 * Construct TreeData<T>.
 */
function getTreeData<T extends TreeNodeData>(
  node: T,
  level: number,
  parent: TreeData<T> | undefined,
  idToTreeData: Map<string, TreeData<T>>,
) {
  return {
    id: node.id,
    name: node.name,
    data: node as T,
    children:
      node.children.map((nodeData) =>
        idToTreeData.get(nodeData.id! as string),
      ) || new Array<TreeData<T>>(),
    level,
    isLeafNode: isLeafNode(node, level),
    isOpen: true,
    parent,
  };
}

/**
 * Returns sub tree list ids of all the children of given treeNode.
 * @param treeNode - node whose subtree ids should be returned.
 * @param subTreeList - empty array for collecting the ids.
 * @param idToTreeData - map of node id to treeData.
 * @returns list of sub tree ids of treeNode.
 */
export function getSubTreeData<T extends TreeNodeData>(
  treeNode: TreeData<T>,
  subTreeList: Array<string>,
  idToTreeData: Map<string, TreeData<T>>,
): Array<string> {
  for (const child of treeNode.children) {
    subTreeList.push(child.id);
    subTreeList = getSubTreeData(
      idToTreeData.get(child.id)!,
      subTreeList,
      idToTreeData,
    );
  }
  return subTreeList;
}

/**
 * Returns if the index is between the visible indexes
 */
export const isWithinIndexRange = (
  index: number,
  firstIndex: number,
  lastIndex: number,
): boolean => {
  if (index < 0) return false;
  return index >= firstIndex && index <= lastIndex;
};

/**
 * Depth First Search (DFS) to find all sub trees that match the search regexps
 * in order.
 *
 * @param node - the tree node to be searched.
 * @param regexps - the search regxps in the exact order of matching.
 * @param index - the index of the search regexps to match against the node.
 * @param searchMatches - the list of matches that gets populated by search.
 */
export function depthFirstSearch<T extends TreeNodeData>(
  node: T,
  regexps: RegExp[],
  index: number,
  searchMatches: SearchTreeMatch[],
): void {
  if (regexps.length === 0 || index < 0 || index >= regexps.length) return;

  // If we matched the last term or had a mismatch, reset the search from the
  // start otherwise advance to the next search term.
  let nextIndex = index + 1;
  if (node.name.match(regexps[index])) {
    if (nextIndex === regexps.length) {
      searchMatches.push({ nodeId: `${node.id}` });
      nextIndex = 0;
    }
  } else {
    nextIndex = 0;
  }

  node.children.forEach((child) => {
    depthFirstSearch(child, regexps, nextIndex, searchMatches);
  });
}

/**
 * Gets background to the node based on the treeData attribute.
 */
export function getNodeBackgroundColor(
  colors?: TreeNodeColors,
  isDeepLinked?: boolean,
  isActiveSelection?: boolean,
  isSearchMatch?: boolean,
) {
  if (isDeepLinked) return colors?.deepLinkBackgroundColor ?? teal[100];
  if (isActiveSelection)
    return colors?.activeSelectionBackgroundColor ?? deepOrange[300];
  if (isSearchMatch) return colors?.searchMatchBackgroundColor ?? yellow[400];
  return '';
}
