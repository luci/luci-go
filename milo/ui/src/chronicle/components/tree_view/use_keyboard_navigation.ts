// Copyright 2025 The LUCI Authors.
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

import { KeyboardEvent, useCallback } from 'react';

import { FlatTreeItem, Graph, transitiveDescendants } from './build_tree';

const isNavigationKey = (key: string) =>
  [
    'j',
    'J',
    'ArrowDown',
    'k',
    'K',
    'ArrowUp',
    'h',
    'ArrowLeft',
    'Home',
    'End',
  ].includes(key);
const isExpansionKey = (key: string) =>
  ['o', 'O', 'Enter', ' ', 'H', 'L', 'l', 'ArrowRight'].includes(key);

export function useKeyboardNavigation(
  visibleItems: FlatTreeItem[],
  selectedKey: string | null,
  setSelectedKey: (key: string | null) => void,
  expandedIds: Set<string>,
  setExpandedIds: (ids: Set<string>) => void,
  graph: Graph,
) {
  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.ctrlKey || e.altKey || e.metaKey) return;

      const currentIndex = visibleItems.findIndex(
        (item) => item.key === selectedKey,
      );
      if (currentIndex === -1 && visibleItems.length > 0) {
        setSelectedKey(visibleItems[0].key);
        return;
      }

      const currentItem = visibleItems[currentIndex];

      if (isNavigationKey(e.key)) {
        e.preventDefault();
        handleNavigation(
          currentIndex,
          visibleItems,
          setSelectedKey,
          currentItem,
          e.key,
          e.shiftKey,
        );
      } else if (isExpansionKey(e.key)) {
        e.preventDefault();
        handleExpansion(
          e.key,
          e.shiftKey,
          currentIndex,
          visibleItems,
          setSelectedKey,
          expandedIds,
          setExpandedIds,
          graph,
        );
      }
    },
    [
      visibleItems,
      selectedKey,
      expandedIds,
      graph,
      setExpandedIds,
      setSelectedKey,
    ],
  );

  return { handleKeyDown };
}

const scrollSelectedIntoView = (index: number) => {
  const element = document.getElementById(`tree-row-${index}`);
  if (element) {
    element.scrollIntoView({ block: 'nearest' });
  }
};

const handleExpansion = (
  key: string,
  shiftKey: boolean,
  currentIndex: number,
  visibleItems: FlatTreeItem[],
  setSelectedKey: (key: string | null) => void,
  expandedIds: Set<string>,
  setExpandedIds: (ids: Set<string>) => void,
  graph: Graph,
) => {
  const currentItem = visibleItems[currentIndex];
  let newIndex = currentIndex;

  const updateRecursiveState = (
    startId: string,
    shouldExpand: boolean,
    baseSet: Set<string>,
  ): Set<string> => {
    const descendants = transitiveDescendants(graph, startId);
    const nextSet = new Set(baseSet);
    if (shouldExpand) {
      nextSet.add(startId);
      descendants.forEach((id) => nextSet.add(id));
    } else {
      nextSet.delete(startId);
      descendants.forEach((id) => nextSet.delete(id));
    }
    return nextSet;
  };

  switch (key) {
    // Toggle collapse / collapse all
    case 'O':
    case 'o':
    case 'Enter':
    case ' ': {
      // handle shift / O case as Toggle collapse/expand all
      if (shiftKey || 'O' === key) {
        if (currentItem) {
          const isCurrentlyExpanded = expandedIds.has(currentItem.id);
          setExpandedIds(
            updateRecursiveState(
              currentItem.id,
              !isCurrentlyExpanded,
              expandedIds,
            ),
          );
        }
        return;
      }
      if (currentItem) {
        const next = new Set(expandedIds);
        if (next.has(currentItem.id)) {
          next.delete(currentItem.id);
        } else {
          next.add(currentItem.id);
        }
        setExpandedIds(next);
      }
      return;
    }

    // Go up and collapse
    case 'H':
      if (!currentItem) break;
      let newItem = currentItem;
      if (currentItem.parentId) {
        for (let i = currentIndex - 1; i >= 0; i--) {
          if (visibleItems[i].depth === currentItem.depth - 1) {
            setSelectedKey(visibleItems[i].key);
            scrollSelectedIntoView(i);
            newItem = visibleItems[i];
            break;
          }
        }
      }
      setExpandedIds(updateRecursiveState(newItem.id, false, expandedIds));
      return;

    // Go down and expand all
    case 'L':
      if (currentItem && currentItem.hasChildren) {
        setExpandedIds(updateRecursiveState(currentItem.id, true, expandedIds));
      }
      return;
    case 'l':
    case 'ArrowRight': {
      if (!currentItem) return;
      if (!expandedIds.has(currentItem.id) && currentItem.hasChildren) {
        const next = new Set(expandedIds);
        next.add(currentItem.id);
        setExpandedIds(next);
      } else if (
        expandedIds.has(currentItem.id) &&
        currentIndex + 1 < visibleItems.length
      ) {
        newIndex = currentIndex + 1;
      }
      break;
    }
  }
  if (newIndex !== currentIndex && visibleItems[newIndex]) {
    setSelectedKey(visibleItems[newIndex].key);
    scrollSelectedIntoView(newIndex);
  }
};

const handleNavigation = (
  currentIndex: number,
  visibleItems: FlatTreeItem[],
  setSelectedKey: (key: string | null) => void,
  currentItem: FlatTreeItem,
  key: string,
  shiftKey: boolean,
) => {
  let newIndex = currentIndex;
  switch (key) {
    // Go down (shift+j on current level)
    case 'j':
    case 'J':
    case 'ArrowDown':
      if (shiftKey || key === 'J') {
        for (let i = currentIndex + 1; i < visibleItems.length; i++) {
          if (visibleItems[i].depth < currentItem.depth) break;
          if (visibleItems[i].depth === currentItem.depth) {
            newIndex = i;
            break;
          }
        }
      } else {
        newIndex = Math.min(visibleItems.length - 1, currentIndex + 1);
      }
      break;

    // Go up (shift+k on current level)
    case 'k':
    case 'K':
    case 'ArrowUp':
      if (shiftKey || key === 'K') {
        for (let i = currentIndex - 1; i >= 0; i--) {
          if (visibleItems[i].depth < currentItem.depth) break;
          if (visibleItems[i].depth === currentItem.depth) {
            newIndex = i;
            break;
          }
        }
      } else {
        newIndex = Math.max(0, currentIndex - 1);
      }
      break;
    // Go up without collapsing
    case 'h':
    case 'ArrowLeft': {
      if (!currentItem || !currentItem.parentId) return;

      // Always go to parent, never collapse the current node
      for (let i = currentIndex - 1; i >= 0; i--) {
        if (visibleItems[i].depth === currentItem.depth - 1) {
          newIndex = i;
          break;
        }
      }
      break;
    }
    // Go to top
    case 'Home':
      newIndex = 0;
      break;

    // Go to bottom
    case 'End':
      newIndex = visibleItems.length - 1;
      break;
  }
  if (newIndex !== currentIndex && visibleItems[newIndex]) {
    setSelectedKey(visibleItems[newIndex].key);
    scrollSelectedIntoView(newIndex);
  }
};
