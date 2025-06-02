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

export function getSubTreeData<T extends TreeNodeData>(
  treeNode: TreeData<T>,
): Array<string> {
  const subTreeList: Array<string> = [];
  function collectDescendantIds(node: TreeData<T>, list: string[]) {
    for (const childNode of node.children) {
      list.push(childNode.id);
      collectDescendantIds(childNode, list);
    }
  }
  collectDescendantIds(treeNode, subTreeList);
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

// Helper to recursively copy a node and its children, ensuring string IDs
// This is essentially your existing copyTreeHelper
function cloneNodeWithStringId<T extends TreeNodeData>(node: T): T {
  const clonedNode = { ...node, id: String(node.id) };
  if (node.children && node.children.length > 0) {
    clonedNode.children = node.children.map((child) =>
      cloneNodeWithStringId(child as T),
    );
  } else {
    clonedNode.children = []; // Ensure children array exists even if empty
  }
  return clonedNode;
}

function buildTreeDataRecursive<T extends TreeNodeData>(
  originalNode: T,
  level: number,
  parentTreeData: TreeData<T> | undefined,
  flatList: Array<TreeData<T>>, // Accumulator for the flat list
): TreeData<T> {
  // Create the version of the node that will go into TreeData.data
  // This ensures TreeData.data contains nodes with string IDs and copied children
  const dataNode = cloneNodeWithStringId(originalNode);

  const currentTreeDataObject: TreeData<T> = {
    id: dataNode.id, // Already a string from cloneNodeWithStringId
    name: dataNode.name,
    data: dataNode,
    level: level,
    parent: parentTreeData,
    children: [], // Child TreeData objects will be added here
    isOpen: true, // Default state
    // Your existing isLeafNode logic:
    isLeafNode: !(originalNode.children && originalNode.children.length > 0),
  };

  flatList.push(currentTreeDataObject);

  if (originalNode.children && originalNode.children.length > 0) {
    for (const childOriginalNode of originalNode.children as T[]) {
      const childTreeDataObject = buildTreeDataRecursive(
        childOriginalNode,
        level + 1,
        currentTreeDataObject, // Pass current TreeData as parent
        flatList,
      );
      currentTreeDataObject.children.push(childTreeDataObject);
    }
  }
  return currentTreeDataObject;
}

export function generateTreeDataList<T extends TreeNodeData>(
  root: readonly T[],
): Array<TreeData<T>> {
  const resultFlatList: Array<TreeData<T>> = [];
  for (const originalRootNode of root) {
    // The root TreeData objects don't need to be captured from return of buildTreeDataRecursive
    // if only the flat list is the desired output.
    buildTreeDataRecursive(originalRootNode, 0, undefined, resultFlatList);
  }
  return resultFlatList;
}
