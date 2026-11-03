// Copyright 2026 The LUCI Authors.
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

import {
  startTransition,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useDebounce } from 'react-use';

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import { getBaseNodeId } from '../../utils/id';
import { SearchIndexWorkerResponse } from '../../utils/search_index_worker';
// ?worker&url is special vite syntax to import a web worker script
// and retrieve the URL to the script.
import searchIndexWorkerUrl from '../../utils/search_index_worker?worker&url';
import { getTrustedWorkerURL } from '../../utils/worker_utils';

const DEFAULT_MATCH_NAV_DEBOUNCE_MS = 300;
const SEARCH_QUERY_DEBOUNCE_MS = 300;
const EMPTY_SEARCH_INDEX_MAP: ReadonlyMap<string, string> = new Map();

export interface SearchableNode {
  id: string;
  label: string;
  view?: Check | Stage;
}

export interface UseNodeSearchOptions {
  /**
   * Nodes ordered in the sequence that Next/Previous should iterate through matches.
   */
  nodes: SearchableNode[];
  /**
   * Optional map of ValueRef digests to ValueData payloads to include in the search index.
   */
  valueDataMap?: ReadonlyMap<string, ValueData>;
  /**
   * Whether the underlying view data is still loading.
   */
  isLoading?: boolean;
  /**
   * Currently selected node ID (typically from ChronicleContext).
   */
  selectedNodeId: string | undefined;
  /**
   * Setter for the selected node ID (typically from ChronicleContext).
   */
  setSelectedNodeId: (id: string | undefined) => void;
  /**
   * Optional callback invoked whenever search settles on a matched node—both when typing
   * pauses after query debounce and when rapid Next/Previous/Enter match stepping settles
   * (e.g. to trigger fitView in GraphView or scroll-into-view in TreeView/TimelineView
   * without firing on manual node clicks).
   */
  onSettledMatch?: (targetId: string) => void;
  /**
   * Debounce duration (in ms) before committing `effectiveSettledNodeId` and calling
   * `onSettledMatch` during rapid Next/Previous clicks. Defaults to 300ms.
   */
  settleDebounceMs?: number;
}

/**
 * Returns the index of the given node ID within the matches list (or precomputed match index Map)
 * by comparing either the direct node ID or its base ID (without prefix). Returns -1 if not found.
 */
export function getNodeMatchIndex(
  id: string | undefined,
  matches: readonly string[] | Map<string, number>,
): number {
  if (!id) return -1;
  if (matches instanceof Map) {
    const directIndex = matches.get(id);
    if (directIndex !== undefined) return directIndex;
    const baseId = getBaseNodeId(id, { includePrefix: false });
    return (baseId ? matches.get(baseId) : undefined) ?? -1;
  }
  if (matches.length === 0) return -1;
  const directIndex = matches.indexOf(id);
  if (directIndex !== -1) return directIndex;
  const baseId = getBaseNodeId(id, { includePrefix: false });
  return baseId ? matches.indexOf(baseId) : -1;
}

/**
 * Checks whether the given node ID matches any ID in the matches list,
 * comparing both the direct node ID and its base ID (without prefix).
 */
export function isNodeInMatches(
  id: string | undefined,
  matches: readonly string[] | Map<string, number>,
): boolean {
  return getNodeMatchIndex(id, matches) !== -1;
}

/**
 * Builds a Map from node ID (and base ID without prefix) to its index in `matches`
 * for O(1) match index lookups.
 */
export function buildMatchIndexMap(
  matches: readonly string[],
): Map<string, number> {
  const matchIndexMap = new Map<string, number>();
  for (let i = 0; i < matches.length; i++) {
    const id = matches[i];
    if (!matchIndexMap.has(id)) {
      matchIndexMap.set(id, i);
    }
    const baseId = getBaseNodeId(id, { includePrefix: false });
    if (baseId && !matchIndexMap.has(baseId)) {
      matchIndexMap.set(baseId, i);
    }
  }
  return matchIndexMap;
}

/**
 * Finds the index in `matches` of the next matching node that appears after
 * `selectedId` in `nodes`. If `selectedId` is not found or no match appears after
 * it, wraps around to the first match (index 0).
 */
export function findNextMatchIndex(
  selectedId: string | undefined,
  nodes: readonly SearchableNode[],
  matches: readonly string[],
): number {
  if (matches.length === 0) return 0;
  if (!selectedId) return 0;
  const baseSelectedId = getBaseNodeId(selectedId, { includePrefix: false });
  const selectedNodeIndex = nodes.findIndex(
    (n) => n.id === selectedId || (baseSelectedId && n.id === baseSelectedId),
  );
  if (selectedNodeIndex === -1) return 0;

  const matchIndexMap = buildMatchIndexMap(matches);
  for (let i = selectedNodeIndex + 1; i < nodes.length; i++) {
    const matchIdx = getNodeMatchIndex(nodes[i].id, matchIndexMap);
    if (matchIdx !== -1) return matchIdx;
  }
  return 0;
}

export interface UseNodeSearchResult {
  searchQuery: string;
  matchedNodeIds: string[];
  /**
   * Non-negative, bounds-clamped index into `matchedNodeIds` (`0 <= safeMatchIndex < matchedNodeIds.length`
   * when matches exist, or `0` when empty). Unlike `selectedMatchIndex` (which is `-1` when no match is
   * selected), this always points to the active match or falls back to the last navigated match (or `0`).
   * It is always safe to index `matchedNodeIds[safeMatchIndex]` and render as `safeMatchIndex + 1`.
   */
  safeMatchIndex: number;
  /**
   * Whether a matching node is currently selected (specifically the node at `safeMatchIndex`).
   * False when a non-matching node is selected or no node is selected, in which case the UI
   * displays "Match ? of Y" and pressing Next/Previous restores selection to `matchedNodeIds[safeMatchIndex]`.
   */
  isCurrentMatchSelected: boolean;
  /**
   * The node ID that has settled after any rapid Next/Previous clicking finishes.
   *
   * Consumers should use `effectiveSettledNodeId` (rather than `selectedNodeId`) to drive
   * heavy side-effects and expensive UI renders—such as the Inspector panel and graph
   * neighbor/edge highlighting—so rapid match iteration stays responsive.
   */
  effectiveSettledNodeId: string | undefined;
  /**
   * Whether the background search index worker has finished indexing the nodes.
   */
  isIndexReady: boolean;
  /**
   * Handles user input in the search box, updating matching nodes and selection.
   */
  handleSearchChange: (query: string) => void;
  /**
   * Clears the active search query, matching nodes, and search selection.
   */
  handleClearSearch: () => void;
  /**
   * Advances to the next matching node (or returns to the current match if off-match).
   */
  handleNextMatch: () => void;
  /**
   * Steps to the previous matching node (or returns to the current match if off-match).
   */
  handlePrevMatch: () => void;
  /**
   * Steps directly to a specific matched node ID with two-phase navigation debounce.
   */
  stepToMatch: (targetId: string) => void;
  /**
   * Cancels any pending match navigation debounce timer.
   */
  cancelPendingMatchNav: () => void;
  /**
   * Cancels pending navigation and resets match tracking to the current selection
   * (e.g. when changing workflow views or critical path filter).
   */
  resetSearchTracking: () => void;
}

/**
 * Reusable hook for filtering nodes by search query and iterating through matches
 * with Next/Previous controls across Chronicle views (GraphView, TreeView, TimelineView).
 *
 * Implements a two-phase update pattern to keep the UI responsive during rapid Next/Previous
 * navigation:
 * 1. Immediate phase: Immediately updates the active match and `safeMatchIndex` so lightweight UI
 *    (like the "Match X of Y" counter) responds instantaneously, and schedules `selectedNodeId`
 *    inside a transition.
 * 2. Settled phase: Freezes `effectiveSettledNodeId` during rapid clicks and commits it (along
 *    with calling `onSettledMatch`) only after clicking pauses. Consuming components should bind
 *    heavy side-effects—such as rendering the Inspector panel or recomputing graph neighbor/edge
 *    highlights—to `effectiveSettledNodeId` rather than raw selection state.
 */
export function useNodeSearch({
  nodes,
  valueDataMap,
  isLoading = false,
  selectedNodeId,
  setSelectedNodeId,
  onSettledMatch,
  settleDebounceMs = DEFAULT_MATCH_NAV_DEBOUNCE_MS,
}: UseNodeSearchOptions): UseNodeSearchResult {
  const [searchQuery, setSearchQuery] = useState('');

  // State and refs to decouple immediate match stepping from expensive display updates
  // while rapidly clicking Next/Previous.
  const [settledSelectedNodeId, setSettledSelectedNodeId] = useState<
    string | undefined
  >(selectedNodeId);
  const [activeMatchNodeId, setActiveMatchNodeId] = useState<
    string | undefined
  >(undefined);
  const [isMatchNavPending, setIsMatchNavPending] = useState(false);
  const isMatchNavPendingRef = useRef(false);
  const selectedNodeIdRef = useRef(selectedNodeId);
  const prevSelectedNodeIdRef = useRef(selectedNodeId);

  // The last search result node navigated or matched to. Preserved when non-matching
  // nodes are selected or canvas is clicked so Next/Prev or query refinement returns to it.
  const lastMatchNodeIdRef = useRef<string | undefined>(undefined);
  const matchNavTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  const normalizedQuery = searchQuery.trim().toLowerCase();

  const clearMatchNavTimer = useCallback(() => {
    if (matchNavTimerRef.current) {
      clearTimeout(matchNavTimerRef.current);
      matchNavTimerRef.current = undefined;
    }
    isMatchNavPendingRef.current = false;
    setIsMatchNavPending(false);
    setActiveMatchNodeId(undefined);
  }, []);

  const cancelPendingMatchNav = useCallback(() => {
    clearMatchNavTimer();
  }, [clearMatchNavTimer]);

  const resetSearchTracking = useCallback(() => {
    clearMatchNavTimer();
    lastMatchNodeIdRef.current = selectedNodeIdRef.current;
  }, [clearMatchNavTimer]);

  useEffect(() => {
    return () => {
      if (matchNavTimerRef.current) {
        clearTimeout(matchNavTimerRef.current);
      }
    };
  }, []);

  // The node ID that has settled after any rapid Next/Previous clicking finishes;
  // drives full highlighting and the InspectorPanel.
  const effectiveSettledNodeId = isMatchNavPending
    ? settledSelectedNodeId
    : selectedNodeId;

  // The immediate node ID reflecting the latest Next/Previous click; drives the
  // "Match X of Y" counter without waiting for the debounce window.
  const displaySelectedNodeId = activeMatchNodeId ?? selectedNodeId;

  // Search index built asynchronously by the background Web Worker for a specific
  // (nodes, valueDataMap) pair.
  const [indexState, setIndexState] = useState<{
    nodes: SearchableNode[];
    valueDataMap?: ReadonlyMap<string, ValueData>;
    searchIndexMap: ReadonlyMap<string, string>;
  }>(() => ({
    nodes: [],
    valueDataMap: undefined,
    searchIndexMap: EMPTY_SEARCH_INDEX_MAP,
  }));

  const isIndexReady =
    nodes.length === 0 ||
    (indexState.nodes === nodes && indexState.valueDataMap === valueDataMap);
  const searchIndexMap = isIndexReady
    ? indexState.searchIndexMap
    : EMPTY_SEARCH_INDEX_MAP;

  // Precompute the lowercase search text for each node on a background Web Worker thread
  // whenever nodes or valueDataMap change, so the index is ready when the user types in
  // the search box without blocking the main thread.
  useEffect(() => {
    if (nodes.length === 0 || typeof Worker === 'undefined') {
      return;
    }

    let worker: Worker;
    try {
      worker = new Worker(getTrustedWorkerURL(searchIndexWorkerUrl) as URL, {
        type: 'module',
      });
    } catch {
      return;
    }

    worker.onmessage = (e: MessageEvent<SearchIndexWorkerResponse>) => {
      setIndexState({
        nodes,
        valueDataMap,
        searchIndexMap:
          e.data?.searchIndexMap instanceof Map
            ? e.data.searchIndexMap
            : EMPTY_SEARCH_INDEX_MAP,
      });
      worker.terminate();
    };

    worker.postMessage({
      nodes,
      valueDataMap,
    });

    return () => {
      worker.terminate();
    };
  }, [nodes, valueDataMap]);

  const matchedNodeIds = useMemo(() => {
    if (!normalizedQuery || !isIndexReady) {
      return [];
    }
    return nodes
      .filter((node) => searchIndexMap.get(node.id)?.includes(normalizedQuery))
      .map((node) => node.id);
  }, [nodes, normalizedQuery, isIndexReady, searchIndexMap]);

  const selectedMatchIndex = useMemo(() => {
    if (!displaySelectedNodeId || matchedNodeIds.length === 0) {
      return -1;
    }
    return getNodeMatchIndex(displaySelectedNodeId, matchedNodeIds);
  }, [displaySelectedNodeId, matchedNodeIds]);

  const isCurrentMatchSelected = selectedMatchIndex !== -1;

  // Always within [0, matchedNodeIds.length - 1] (or 0 when empty), avoiding -1 when
  // no match is selected and falling back to lastMatchNodeId's index (or 0) if a non-matching
  // or empty selection is active.
  const safeMatchIndex = useMemo(() => {
    if (matchedNodeIds.length === 0) return 0;
    if (selectedMatchIndex !== -1) return selectedMatchIndex;
    const lastIdx = getNodeMatchIndex(
      lastMatchNodeIdRef.current,
      matchedNodeIds,
    );
    return lastIdx !== -1 ? lastIdx : 0;
  }, [matchedNodeIds, selectedMatchIndex]);

  // Sync external selection changes (e.g. manual canvas clicks or deselecting) with refs.
  useEffect(() => {
    if (selectedNodeId !== prevSelectedNodeIdRef.current) {
      prevSelectedNodeIdRef.current = selectedNodeId;
      selectedNodeIdRef.current = selectedNodeId;

      if (
        isMatchNavPendingRef.current &&
        selectedNodeId !== activeMatchNodeId
      ) {
        clearMatchNavTimer();
      }

      if (selectedNodeId && isNodeInMatches(selectedNodeId, matchedNodeIds)) {
        lastMatchNodeIdRef.current = selectedNodeId;
      }
    }
  }, [selectedNodeId, activeMatchNodeId, clearMatchNavTimer, matchedNodeIds]);

  const prevNodesRef = useRef(nodes);

  // Automatically update selection when the graph's nodes update (e.g. after fetching an updated
  // workplan) while a search query is active.
  useEffect(() => {
    if (isLoading || !isIndexReady || nodes.length === 0) {
      return;
    }
    const nodesChanged = nodes !== prevNodesRef.current;
    prevNodesRef.current = nodes;

    if (!nodesChanged || !normalizedQuery) {
      return;
    }

    const selected = selectedNodeIdRef.current;
    const isManualSelection =
      selected !== undefined && selected !== lastMatchNodeIdRef.current;

    // Do not hijack manual selection on the canvas when nodes update.
    if (isManualSelection) {
      return;
    }

    if (matchedNodeIds.length > 0) {
      if (!selected || !isNodeInMatches(selected, matchedNodeIds)) {
        const nextId =
          lastMatchNodeIdRef.current &&
          isNodeInMatches(lastMatchNodeIdRef.current, matchedNodeIds)
            ? lastMatchNodeIdRef.current
            : matchedNodeIds[0];
        lastMatchNodeIdRef.current = nextId;
        selectedNodeIdRef.current = nextId;
        setSelectedNodeId(nextId);
        onSettledMatch?.(nextId);
      }
    } else if (selectedNodeIdRef.current) {
      lastMatchNodeIdRef.current = undefined;
      selectedNodeIdRef.current = undefined;
      setSelectedNodeId(undefined);
    }
  }, [
    isLoading,
    isIndexReady,
    nodes,
    normalizedQuery,
    matchedNodeIds,
    setSelectedNodeId,
    onSettledMatch,
  ]);

  // Invoke onSettledMatch when typing pauses after debounce.
  useDebounce(
    () => {
      const selected = selectedNodeIdRef.current;
      if (selected && isNodeInMatches(selected, matchedNodeIds)) {
        onSettledMatch?.(selected);
      }
    },
    SEARCH_QUERY_DEBOUNCE_MS,
    [normalizedQuery],
  );

  /**
   * Advances selection to `targetId` during search match navigation (`<` / `>` or Enter)
   * using a two-phase update so rapid multi-clicks stay immediately responsive.
   */
  const stepToMatch = useCallback(
    (targetId: string) => {
      if (!isMatchNavPendingRef.current) {
        setSettledSelectedNodeId(selectedNodeIdRef.current);
        isMatchNavPendingRef.current = true;
        setIsMatchNavPending(true);
      }
      lastMatchNodeIdRef.current = targetId;
      setActiveMatchNodeId(targetId);
      startTransition(() => {
        selectedNodeIdRef.current = targetId;
        setSelectedNodeId(targetId);
      });

      if (matchNavTimerRef.current) {
        clearTimeout(matchNavTimerRef.current);
      }
      matchNavTimerRef.current = setTimeout(() => {
        matchNavTimerRef.current = undefined;
        isMatchNavPendingRef.current = false;
        startTransition(() => {
          setSettledSelectedNodeId(targetId);
          setIsMatchNavPending(false);
          setActiveMatchNodeId(undefined);
        });
        onSettledMatch?.(targetId);
      }, settleDebounceMs);
    },
    [setSelectedNodeId, onSettledMatch, settleDebounceMs],
  );

  const handleNextMatch = useCallback(() => {
    if (matchedNodeIds.length === 0) return;
    const isMatch = activeMatchNodeId !== undefined || isCurrentMatchSelected;
    const curIdx = getNodeMatchIndex(
      lastMatchNodeIdRef.current,
      matchedNodeIds,
    );
    // If currently on a match, advance to the next match (wrapping around).
    // If off-match (e.g. non-matching node selected or canvas clicked), return to the last match.
    const nextIdx =
      isMatch && curIdx !== -1
        ? (curIdx + 1) % matchedNodeIds.length
        : Math.max(0, curIdx);
    stepToMatch(matchedNodeIds[nextIdx]);
  }, [activeMatchNodeId, isCurrentMatchSelected, matchedNodeIds, stepToMatch]);

  const handlePrevMatch = useCallback(() => {
    if (matchedNodeIds.length === 0) return;
    const isMatch = activeMatchNodeId !== undefined || isCurrentMatchSelected;
    const curIdx = getNodeMatchIndex(
      lastMatchNodeIdRef.current,
      matchedNodeIds,
    );
    // If currently on a match, step to the previous match (wrapping around).
    // If off-match (e.g. non-matching node selected or canvas clicked), return to the last match.
    const prevIdx =
      isMatch && curIdx !== -1
        ? (curIdx - 1 + matchedNodeIds.length) % matchedNodeIds.length
        : Math.max(0, curIdx);
    stepToMatch(matchedNodeIds[prevIdx]);
  }, [activeMatchNodeId, isCurrentMatchSelected, matchedNodeIds, stepToMatch]);

  const handleClearSearch = useCallback(() => {
    clearMatchNavTimer();
    setSearchQuery('');
    lastMatchNodeIdRef.current = undefined;
    selectedNodeIdRef.current = undefined;
    setSelectedNodeId(undefined);
  }, [clearMatchNavTimer, setSelectedNodeId]);

  const handleSearchChange = useCallback(
    (query: string) => {
      const norm = query.trim().toLowerCase();
      if (!norm) {
        handleClearSearch();
        return;
      }
      clearMatchNavTimer();
      setSearchQuery(query);

      const currentSelected = selectedNodeIdRef.current;
      const isManualSelection =
        currentSelected !== undefined &&
        currentSelected !== lastMatchNodeIdRef.current;

      const matches = isIndexReady
        ? nodes
            .filter((node) => searchIndexMap.get(node.id)?.includes(norm))
            .map((node) => node.id)
        : [];

      if (matches.length === 0) {
        if (!isManualSelection && currentSelected !== undefined) {
          lastMatchNodeIdRef.current = undefined;
          selectedNodeIdRef.current = undefined;
          setSelectedNodeId(undefined);
        }
        return;
      }

      // If current selection is in the new matches, keep it.
      if (currentSelected && isNodeInMatches(currentSelected, matches)) {
        lastMatchNodeIdRef.current = currentSelected;
        return;
      }

      // If current selection was manual and non-matching, keep it without hijacking.
      if (isManualSelection) {
        return;
      }

      // Otherwise, select the last-used match if it still matches, or the next match in graph order.
      const lastMatch = lastMatchNodeIdRef.current;
      const targetId =
        lastMatch && isNodeInMatches(lastMatch, matches)
          ? lastMatch
          : matches[
              findNextMatchIndex(lastMatch ?? currentSelected, nodes, matches)
            ];

      lastMatchNodeIdRef.current = targetId;
      selectedNodeIdRef.current = targetId;
      setSelectedNodeId(targetId);
    },
    [
      clearMatchNavTimer,
      handleClearSearch,
      isIndexReady,
      nodes,
      searchIndexMap,
      setSelectedNodeId,
    ],
  );

  return {
    searchQuery,
    matchedNodeIds,
    safeMatchIndex,
    isCurrentMatchSelected,
    effectiveSettledNodeId,
    isIndexReady,
    handleSearchChange,
    handleClearSearch,
    handleNextMatch,
    handlePrevMatch,
    stepToMatch,
    cancelPendingMatchNav,
    resetSearchTracking,
  };
}
