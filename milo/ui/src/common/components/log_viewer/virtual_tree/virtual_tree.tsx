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

import { isEmpty, isEqual } from 'lodash';
import React, {
  ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { ListRange, Virtuoso, VirtuosoHandle } from 'react-virtuoso';

import {
  SearchOptions,
  SearchTreeMatch,
  TreeData,
  TreeNodeContainerData,
  TreeNodeData,
} from './types';
import {
  depthFirstSearch,
  generateTreeDataList,
  getSubTreeData,
  isWithinIndexRange,
} from './utils';

export const INITIAL_TREE_LEVEL = 0;

export const DEFAULT_NODE_INDENTATION = 32;

export const SEARCH_PATH_SPLITTER = '/';

/**
 * Props for the Virtual Tree Node Container.
 */
export interface VirtualTreeNodeContainerProps<T extends TreeNodeData> {
  data: TreeNodeContainerData<T>;
  index: number;
  style: React.CSSProperties;
}

export interface VirtualTreeNodeActions<T extends TreeNodeData> {
  onNodeSelect?: (node: TreeData<T>) => void;
  onNodeToggle?: (node: TreeData<T>) => void;
  isSelected?: boolean;
  isSearchMatch?: boolean;
  isActiveSelection?: boolean;
}

/**
 * Props for the Virtual Tree.
 */
export interface VirtualTreeProps<T extends TreeNodeData> {
  /* Data Options */
  root: readonly T[];

  /* Renderers */
  collapseIcon?: ReactNode;
  expandIcon?: ReactNode;
  // Custom node renderer for the virtual tree.
  itemContent: (
    index: number,
    row: TreeData<T>,
    context: VirtualTreeNodeActions<T>,
  ) => ReactNode;
  isTreeCollapsed?: boolean;
  disableVirtualization?: boolean;

  /**
   * Node ids that are marked as selected.
   */
  selectedNodes?: Set<string>;

  /* Search */
  searchOptions?: SearchOptions;

  /**
   * The currently active search index with respect to the total matches. If
   * the prop is not passed, navigation will not be supported.
   */
  searchActiveIndex?: number;

  /**
   * Toggle the scroll to the active index.
   */
  scrollToggle?: boolean;

  /**
   * Callback function which returns total search matches when search matches
   * are found. activeIndex is -1 when no search matches are found.
   */
  onSearchMatchFound?: (
    activeIndex: number,
    totalSearchMatches: number,
  ) => void;
  onNodeSelect?: (treeNodeData: T) => void;
  onNodeToggle?: (treeNodeData: T) => void;

  // boolean accessor for activeSelection property of the TreeData which enables
  // rendering of the selected nodes in viewport. If multiple are matched, the
  // first match will rendered in the viewport.
  setActiveSelectionFn?: (treeNodeData: T) => boolean;
}

/**
 * Renders Virtual Tree Component.
 */
export function VirtualTree<T extends TreeNodeData>({
  root,
  searchOptions,
  searchActiveIndex,
  scrollToggle,
  itemContent,
  isTreeCollapsed,
  disableVirtualization,
  selectedNodes,
  onSearchMatchFound,
  setActiveSelectionFn,
  onNodeSelect,
  onNodeToggle,
}: VirtualTreeProps<T>) {
  const [displayIsTreeCollapsed, setDisplayIsTreeExpanded] = useState<
    boolean | undefined
  >(undefined);
  const [displaySearchOptions, setDisplaySearchOptions] = useState<
    SearchOptions | undefined
  >(undefined);
  const [displaySearchIndex, setDisplaySearchIndex] = useState<number>(-1);
  // Index of the first visible tree node in the view port.
  const firstVisibleIndex = useRef<number>(-1);
  // Index of the last visible tree node in the view port.
  const lastVisibleIndex = useRef<number>(-1);
  // Indicates if the node toggle is in progress the tree browser.
  const [isNodeToggleInProgress, setIsNodeToggleInProgress] = useState(false);
  // Reference to virtuoso component.
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  // List of node ids from open tree data which match with the search data.
  const searchMatchesRef = useRef<SearchTreeMatch[]>([]);
  const activeSelectionRef = useRef<string | undefined>(undefined);

  // List of expanded(open) tree data which will render as tree nodes.
  const [openTreeDataList, setOpenTreeDataList] = useState<TreeData<T>[]>([]);

  // List of collapsed(closed) tree data which will not be rendered
  // as tree nodes.
  const closedTreeNodeIdToSubTreeIds = useRef<Map<string, string[]>>(new Map());

  // Triggered everytime the list items change continuously updating
  // the first and last visible index in the list.
  const handleOnRangeChanged = ({ startIndex, endIndex }: ListRange) => {
    firstVisibleIndex.current = startIndex;
    lastVisibleIndex.current = endIndex;
  };

  const { allTreeDataList, idToTreeDataMap, deepLinkIndex } = useMemo(() => {
    const allTreeDataList = generateTreeDataList(root as T[]);

    // Map ObjectNode id to TreeData<T>.
    const idToTreeDataMap: Map<string, TreeData<T>> = new Map(
      allTreeDataList.map((treeData) => [treeData.id.toString(), treeData]),
    );
    setOpenTreeDataList(allTreeDataList);

    let deepLinkIndex = 0;
    for (const [index, treeData] of allTreeDataList.entries()) {
      if (setActiveSelectionFn?.(treeData.data)) {
        deepLinkIndex = index;
        break;
      }
    }

    return { allTreeDataList, idToTreeDataMap, deepLinkIndex };
  }, [root, setActiveSelectionFn]);

  // Figure out the first visible index within the search matches.
  // If none is visible in the current window, reset to 0.
  // If there are no matches it returns -1 as invalid.
  const getFirstVisibleSearchMatchIndex = useCallback(
    (searchMatches: SearchTreeMatch[], treeDataList: TreeData<T>[]) => {
      if (searchMatches.length === 0) {
        return -1;
      }

      const start = firstVisibleIndex.current;
      const end = lastVisibleIndex.current;
      for (let i = start; i <= end; i++) {
        const treeData = treeDataList.at(i);
        const index = searchMatches.findIndex(
          (match) => match.nodeId === treeData?.id,
        );

        if (index >= 0) return index;
      }

      // Reset the index to 0 since none of the matches are in the current window.
      return 0;
    },
    [],
  );

  const getValidRegexps = useCallback(
    (pattern: string, flag: string): RegExp[] => {
      if (pattern.length === 0) return [];

      // Construct a subtree path if there is a path splitter with valid regexps
      // otherwise default to a single node search.
      let isSubtreeValid = true;
      const regexps: RegExp[] = [];
      for (const segment of pattern.split(SEARCH_PATH_SPLITTER)) {
        if (segment.length === 0) continue;
        try {
          regexps.push(new RegExp(segment, flag));
        } catch {
          isSubtreeValid = false;
          break;
        }
      }

      if (isSubtreeValid) return regexps;

      // Invalid subtree so we attempt to treat the pattern as a single node.
      try {
        return [new RegExp(pattern, flag)];
      } catch {
        return [];
      }
    },
    [],
  );

  /**
   * Retrieves all the matching search term in the open tree list. Since
   * the virtual tree is not completely rendered, this method returns all
   * the matching search term nodes to scroll into.
   */
  const getSearchMatches = useCallback(
    (treeDataList: TreeData<T>[]) => {
      const searchMatches: SearchTreeMatch[] = [];
      if (!searchOptions) return searchMatches;

      // Dont trigger the search match if the pattern or expanded tree is empty.
      if (!searchOptions.pattern || treeDataList.length === 0)
        return searchMatches;

      // Split the search term into non empty regexps so the tree can be searched
      // in the exact order specified by the search path.
      const searchFlag = searchOptions.ignoreCase ? 'i' : '';
      const searchRegexps = getValidRegexps(searchOptions.pattern, searchFlag);
      root.forEach((node) =>
        depthFirstSearch(node, searchRegexps, /* index= */ 0, searchMatches),
      );
      return searchMatches;
    },
    [getValidRegexps, root, searchOptions],
  );

  // Finds the first visible search match against the given search options.
  const searchTree = useCallback(
    (treeDataList: TreeData<T>[]) => {
      // Reset the node states every time pattern changes to avoid staleness.
      searchMatchesRef.current = [];
      const matches = getSearchMatches(treeDataList);
      const index = getFirstVisibleSearchMatchIndex(matches, treeDataList);
      searchMatchesRef.current = matches;
      setDisplaySearchIndex(index);
    },
    [getFirstVisibleSearchMatchIndex, getSearchMatches],
  );

  /**
   * Updates the open tree data by rendering only expanded tree nodes while
   * collapsed nodes are removed from the DOM.
   */
  const updateOpenTreeDataList = useCallback(() => {
    const allClosedSubTreeIdsSet = new Set<string>();
    // Iterate over the map values (which are arrays of IDs)
    // and add each ID to the set.
    for (const subTreeIdList of closedTreeNodeIdToSubTreeIds.current.values()) {
      for (const id of subTreeIdList) {
        allClosedSubTreeIdsSet.add(id);
      }
    }

    const openTreeList = allTreeDataList.filter(
      (treeData: TreeData<T>) => !allClosedSubTreeIdsSet.has(treeData.id),
    );
    setOpenTreeDataList(openTreeList);

    if (isNodeToggleInProgress) {
      setIsNodeToggleInProgress(false);
    }
  }, [allTreeDataList, isNodeToggleInProgress]);

  /**
   * Expands the list of nodes provided.
   */
  const expandNodes = useCallback(
    (treeDataList: TreeData<T>[]) => {
      for (const treeData of treeDataList) {
        closedTreeNodeIdToSubTreeIds.current.delete(treeData.id);
        treeData.isOpen = true;
      }
      updateOpenTreeDataList();
    },
    [updateOpenTreeDataList],
  );

  /**
   * Expands all the collapsed nodes in the tree.
   */
  const expandAllNodes = useCallback(() => {
    const nodesToBeExpanded: TreeData<T>[] = [];
    for (const key of closedTreeNodeIdToSubTreeIds.current.keys()) {
      if (idToTreeDataMap.has(key)) {
        nodesToBeExpanded.push(idToTreeDataMap.get(key) as TreeData<T>);
      }
    }
    expandNodes(nodesToBeExpanded);
  }, [expandNodes, idToTreeDataMap]);

  /**
   * Collapses the list of nodes provided.
   */
  const collapseNodes = useCallback(
    (treeDataList: TreeData<T>[]) => {
      for (const treeData of treeDataList) {
        const subTreeDataIdList = getSubTreeData(treeData);
        closedTreeNodeIdToSubTreeIds.current.set(
          treeData.id,
          subTreeDataIdList,
        );
        treeData.isOpen = false;
      }
      updateOpenTreeDataList();
    },
    [updateOpenTreeDataList],
  );

  /**
   * Collapse all nodes in the tree.
   */
  const collapseAllNodes = useCallback(() => {
    const allRootNodes = allTreeDataList.filter(
      (treeNode: TreeData<T>) => !treeNode.isLeafNode,
    );
    collapseNodes(allRootNodes);
  }, [allTreeDataList, collapseNodes]);

  /**
   * Toggles the state of the tree node to open or close.
   */
  const handleNodeToggle = useCallback(
    (treeData: TreeData<T>) => {
      setIsNodeToggleInProgress(true);
      onNodeToggle?.(treeData.data);
      if (treeData.isOpen) {
        collapseNodes([treeData]);
      } else {
        expandNodes([treeData]);
      }
    },
    [collapseNodes, expandNodes, onNodeToggle],
  );

  const handleNodeSelect = (treeData: TreeData<T>) => {
    onNodeSelect?.(treeData.data);
  };

  /**
   * Update parent search states.
   */
  useEffect(() => {
    onSearchMatchFound?.(displaySearchIndex, searchMatchesRef.current.length);
  }, [displaySearchIndex, onSearchMatchFound, searchMatchesRef.current.length]);

  useEffect(() => {
    if (
      searchActiveIndex === undefined ||
      searchActiveIndex < 0 ||
      isEmpty(searchMatchesRef.current)
    ) {
      activeSelectionRef.current = undefined;
      return;
    }

    // Finds the row Id of the selected search index, this is used to scroll to
    // and highlight the currently active entry.
    const index = openTreeDataList.findIndex(
      (node) =>
        node.id === searchMatchesRef.current.at(searchActiveIndex)?.nodeId,
    );
    if (index < 0) return;

    activeSelectionRef.current = openTreeDataList[index].id;
    if (
      !isWithinIndexRange(
        index,
        firstVisibleIndex.current,
        lastVisibleIndex.current,
      )
    ) {
      virtuosoRef.current?.scrollToIndex({
        index,
        align: 'center',
        behavior: 'auto',
      });
    }
  }, [searchActiveIndex, scrollToggle, openTreeDataList]);

  /**
   * Updates the search state every time the search pattern is updated. The
   * tree state is reset by expanding all the nodes and then the search is
   * performed. However, the tree state can be changed post search. If the
   * search pattern is empty, the search state is reset.
   */
  useEffect(() => {
    if (!isEqual(displaySearchOptions, searchOptions)) {
      setDisplaySearchOptions(searchOptions);
      searchTree(allTreeDataList);
    }
  }, [searchOptions, displaySearchOptions, searchTree, allTreeDataList]);

  useEffect(() => {
    if (displayIsTreeCollapsed !== isTreeCollapsed) {
      if (isTreeCollapsed) {
        collapseAllNodes();
      } else {
        expandAllNodes();
      }
      setDisplayIsTreeExpanded(isTreeCollapsed);
    }
  }, [
    collapseAllNodes,
    displayIsTreeCollapsed,
    expandAllNodes,
    isTreeCollapsed,
  ]);

  // Check if the default node renderer needs to be overridden by user provided
  // custom node renderer.
  function generateItemContents(index: number, row: TreeData<T>) {
    return itemContent(index, row, {
      onNodeSelect: handleNodeSelect,
      onNodeToggle: handleNodeToggle,
      isSelected: selectedNodes?.has(row.id),
      isSearchMatch: searchMatchesRef.current.some(
        (match) => match.nodeId === row.id,
      ),
      isActiveSelection:
        activeSelectionRef.current === row.id && !!searchOptions?.pattern,
    });
  }

  return (
    <Virtuoso
      data-testid="virtual-tree"
      ref={virtuosoRef}
      data={openTreeDataList}
      totalCount={openTreeDataList.length}
      rangeChanged={handleOnRangeChanged}
      itemContent={generateItemContents}
      // Setting this to total count disables virtualization
      initialItemCount={disableVirtualization ? openTreeDataList.length : 0}
      key={disableVirtualization ? openTreeDataList.length : undefined}
      // initial top item does not render when virtualization is disabled
      initialTopMostItemIndex={!disableVirtualization ? deepLinkIndex : 0}
    />
  );
}
