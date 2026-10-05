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
   * Optional callback invoked whenever search settles on a matched node—both when a
   * debounced search query auto-selects the initial match and when rapid Next/Previous/Enter
   * match stepping settles (e.g. to trigger fitView in GraphView or scroll-into-view in
   * TreeView/TimelineView without firing on manual node clicks).
   */
  onSettledMatch?: (targetId: string) => void;
  /**
   * Debounce duration (in ms) before committing `effectiveSettledNodeId` and calling
   * `onSettledMatch` during rapid Next/Previous clicks. Defaults to 300ms.
   */
  settleDebounceMs?: number;
}

export interface UseNodeSearchResult {
  searchQuery: string;
  matchedNodeIds: string[];
  /**
   * Non-negative, bounds-clamped index into `matchedNodeIds` (`0 <= safeMatchIndex < matchedNodeIds.length`
   * when matches exist, or `0` when empty). Unlike `selectedMatchIndex` (which is `-1` when no match is
   * selected) or `storedMatchIndex` (which can exceed `matchedNodeIds.length` as the query narrows),
   * this is always safe to index `matchedNodeIds[safeMatchIndex]` and render as `safeMatchIndex + 1`.
   */
  safeMatchIndex: number;
  /**
   * Whether the node at `safeMatchIndex` is currently selected.
   * False when a non-matching node is selected (or selection was cleared after search settled),
   * in which case the UI displays "Match ? of Y" and pressing Next/Previous restores
   * selection to `matchedNodeIds[safeMatchIndex]`.
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
  handleSearchChange: (query: string) => void;
  handleClearSearch: () => void;
  handleNextMatch: () => void;
  handlePrevMatch: () => void;
  stepToMatch: (targetId: string) => void;
  cancelPendingMatchNav: () => void;
  resetSearchTracking: () => void;
}

/**
 * Reusable hook for filtering nodes by search query and iterating through matches
 * with Next/Previous controls across Chronicle views (GraphView, TreeView, TimelineView).
 *
 * Implements a two-phase update pattern to keep the UI responsive during rapid Next/Previous
 * navigation:
 * 1. Immediate phase: Updates `safeMatchIndex` on every click so lightweight UI like the
 *    "Match X of Y" counter responds instantaneously.
 * 2. Settled phase: Debounces committing `selectedNodeId` and `effectiveSettledNodeId`
 *    (and firing `onSettledMatch`) until clicking pauses. Consuming components should bind
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
  const [debouncedSearchQuery, setDebouncedSearchQuery] = useState('');

  // Track the last search query that triggered automatic selection.
  const [processedSearchQuery, setProcessedSearchQuery] = useState<string>('');

  // Track the last valid match index in state so we preserve position if the user deselects
  // or clicks a non-matching node while a search query is active.
  const [storedMatchIndex, setStoredMatchIndex] = useState<number>(0);

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

  // Synchronously tracks the current match index and whether a match is currently selected
  // inside click handlers so rapid consecutive clicks never read stale state.
  const currentMatchIndexRef = useRef<number>(0);
  const isCurrentMatchSelectedRef = useRef<boolean>(false);
  const matchNavTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );

  const normalizedQuery = searchQuery.trim().toLowerCase();

  useDebounce(
    () => {
      setDebouncedSearchQuery(normalizedQuery);
    },
    SEARCH_QUERY_DEBOUNCE_MS,
    [normalizedQuery],
  );

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
    setDebouncedSearchQuery(normalizedQuery);
    setProcessedSearchQuery(normalizedQuery);
  }, [clearMatchNavTimer, normalizedQuery]);

  // If selectedNodeId changes externally (e.g. user clicks a node on the canvas)
  // while a match navigation debounce is pending, cancel the pending navigation.
  useEffect(() => {
    if (selectedNodeId !== prevSelectedNodeIdRef.current) {
      prevSelectedNodeIdRef.current = selectedNodeId;
      if (
        isMatchNavPendingRef.current &&
        selectedNodeId !== activeMatchNodeId
      ) {
        clearMatchNavTimer();
      }
    }
  }, [selectedNodeId, activeMatchNodeId, clearMatchNavTimer]);

  const resetSearchTracking = useCallback(() => {
    clearMatchNavTimer();
    setDebouncedSearchQuery(normalizedQuery);
    setProcessedSearchQuery('');
    setStoredMatchIndex(0);
    currentMatchIndexRef.current = 0;
    isCurrentMatchSelectedRef.current = false;
  }, [clearMatchNavTimer, normalizedQuery]);

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
    const directIndex = matchedNodeIds.indexOf(displaySelectedNodeId);
    if (directIndex !== -1) {
      return directIndex;
    }
    const baseSelectedId = getBaseNodeId(displaySelectedNodeId, {
      includePrefix: false,
    });
    return baseSelectedId ? matchedNodeIds.indexOf(baseSelectedId) : -1;
  }, [displaySelectedNodeId, matchedNodeIds]);

  const isSearchAutoSelectPending =
    Boolean(normalizedQuery) &&
    normalizedQuery !== processedSearchQuery &&
    !displaySelectedNodeId;

  const isCurrentMatchSelected =
    matchedNodeIds.length > 0 &&
    (selectedMatchIndex !== -1 || isSearchAutoSelectPending);

  // Always within [0, matchedNodeIds.length - 1] (or 0 when empty), avoiding -1 when
  // no match is selected and clamping storedMatchIndex if the match list shrinks.
  const safeMatchIndex =
    matchedNodeIds.length > 0
      ? selectedMatchIndex !== -1
        ? selectedMatchIndex
        : Math.min(storedMatchIndex, matchedNodeIds.length - 1)
      : 0;

  // Sync refs with committed state in an effect so event handlers always have
  // up-to-date values without reading or writing refs during render.
  useEffect(() => {
    selectedNodeIdRef.current = selectedNodeId;
    if (selectedMatchIndex !== -1 && selectedMatchIndex !== storedMatchIndex) {
      setStoredMatchIndex(selectedMatchIndex);
    }
    if (!isMatchNavPendingRef.current) {
      currentMatchIndexRef.current = safeMatchIndex;
      isCurrentMatchSelectedRef.current = isCurrentMatchSelected;
    }
  }, [
    selectedNodeId,
    selectedMatchIndex,
    storedMatchIndex,
    safeMatchIndex,
    isCurrentMatchSelected,
  ]);

  const prevNodesRef = useRef(nodes);

  // Automatically select the first match when search query and background index settle,
  // or when the searchable nodes update with an active search query.
  useEffect(() => {
    if (isLoading || !isIndexReady || nodes.length === 0) {
      return;
    }
    const nodesChanged = nodes !== prevNodesRef.current;
    prevNodesRef.current = nodes;

    if (!nodesChanged && debouncedSearchQuery === processedSearchQuery) {
      return;
    }
    setProcessedSearchQuery(debouncedSearchQuery);

    if (!debouncedSearchQuery) {
      return;
    }
    if (matchedNodeIds.length > 0) {
      const isAlreadyMatched =
        selectedNodeId !== undefined &&
        (matchedNodeIds.includes(selectedNodeId) ||
          matchedNodeIds.includes(
            getBaseNodeId(selectedNodeId, { includePrefix: false }) ?? '',
          ));
      if (!isAlreadyMatched) {
        setStoredMatchIndex(0);
        currentMatchIndexRef.current = 0;
        isCurrentMatchSelectedRef.current = true;
        setSelectedNodeId(matchedNodeIds[0]);
        onSettledMatch?.(matchedNodeIds[0]);
      }
    } else if (selectedNodeId) {
      setSelectedNodeId(undefined);
    }
  }, [
    isLoading,
    isIndexReady,
    nodes,
    debouncedSearchQuery,
    processedSearchQuery,
    matchedNodeIds,
    selectedNodeId,
    setSelectedNodeId,
    onSettledMatch,
  ]);

  /**
   * Advances selection to `targetId` during search match navigation (`<` / `>` or Enter)
   * using a two-phase update so rapid multi-clicks stay immediately responsive:
   *
   * 1. Immediate (urgent) phase:
   *    - Updates `activeMatchNodeId` synchronously so lightweight UI (such as the
   *      "Match X of Y" counter) reflects the new target on every single click.
   *    - Freezes `settledSelectedNodeId` at the node that was selected prior to the
   *      navigation burst (`isMatchNavPending = true`), preventing full graph/tree
   *      re-highlighting on intermediate clicks.
   *    - Schedules the shared `setSelectedNodeId(targetId)` inside `startTransition`
   *      so heavier consumers (like `InspectorPanel`) can be interrupted if another
   *      click arrives before they finish rendering.
   *
   * 2. Settled (debounced) phase:
   *    - Once `settleDebounceMs` (default 300ms) elapses with no further clicks,
   *      commits `targetId` to `settledSelectedNodeId` (triggering full node/edge
   *      highlighting) and invokes `onSettledMatch` if provided.
   */
  const stepToMatch = useCallback(
    (targetId: string) => {
      if (!isMatchNavPendingRef.current) {
        setSettledSelectedNodeId(selectedNodeIdRef.current);
        isMatchNavPendingRef.current = true;
        setIsMatchNavPending(true);
      }
      setDebouncedSearchQuery(normalizedQuery);
      setProcessedSearchQuery(normalizedQuery);
      isCurrentMatchSelectedRef.current = true;
      setActiveMatchNodeId(targetId);
      startTransition(() => {
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
    [normalizedQuery, setSelectedNodeId, onSettledMatch, settleDebounceMs],
  );

  const handleNextMatch = useCallback(() => {
    if (matchedNodeIds.length === 0) return;
    const baseIndex = Math.min(
      Math.max(currentMatchIndexRef.current, 0),
      matchedNodeIds.length - 1,
    );
    // If the stored match is not currently selected (e.g. a non-search-result node is selected),
    // pressing Next returns to the previously-selected match rather than advancing past it.
    const nextIndex = isCurrentMatchSelectedRef.current
      ? (baseIndex + 1) % matchedNodeIds.length
      : baseIndex;
    currentMatchIndexRef.current = nextIndex;
    setStoredMatchIndex(nextIndex);
    stepToMatch(matchedNodeIds[nextIndex]);
  }, [matchedNodeIds, stepToMatch]);

  const handlePrevMatch = useCallback(() => {
    if (matchedNodeIds.length === 0) return;
    const baseIndex = Math.min(
      Math.max(currentMatchIndexRef.current, 0),
      matchedNodeIds.length - 1,
    );
    // If the stored match is not currently selected (e.g. a non-search-result node is selected),
    // pressing Prev returns to the previously-selected match rather than stepping past it.
    const prevIndex = isCurrentMatchSelectedRef.current
      ? (baseIndex - 1 + matchedNodeIds.length) % matchedNodeIds.length
      : baseIndex;
    currentMatchIndexRef.current = prevIndex;
    setStoredMatchIndex(prevIndex);
    stepToMatch(matchedNodeIds[prevIndex]);
  }, [matchedNodeIds, stepToMatch]);

  const handleSearchChange = useCallback(
    (query: string) => {
      clearMatchNavTimer();
      setSearchQuery(query);
      const nextNormalizedQuery = query.trim().toLowerCase();
      if (!nextNormalizedQuery) {
        setDebouncedSearchQuery('');
        setProcessedSearchQuery('');
        setStoredMatchIndex(0);
        currentMatchIndexRef.current = 0;
        isCurrentMatchSelectedRef.current = false;
        setSelectedNodeId(undefined);
        return;
      }

      isCurrentMatchSelectedRef.current = true;
      if (isIndexReady) {
        const currentSelectedId = selectedNodeIdRef.current;
        const baseSelectedId = currentSelectedId
          ? getBaseNodeId(currentSelectedId, { includePrefix: false })
          : undefined;
        const stillMatches = Boolean(
          currentSelectedId &&
            (searchIndexMap
              .get(currentSelectedId)
              ?.includes(nextNormalizedQuery) ||
              (baseSelectedId &&
                searchIndexMap
                  .get(baseSelectedId)
                  ?.includes(nextNormalizedQuery))),
        );

        if (!stillMatches) {
          setStoredMatchIndex(0);
          currentMatchIndexRef.current = 0;
          setSelectedNodeId(undefined);
        }
      }
    },
    [clearMatchNavTimer, isIndexReady, searchIndexMap, setSelectedNodeId],
  );

  const handleClearSearch = useCallback(() => {
    clearMatchNavTimer();
    setSearchQuery('');
    setDebouncedSearchQuery('');
    setProcessedSearchQuery('');
    setStoredMatchIndex(0);
    currentMatchIndexRef.current = 0;
    isCurrentMatchSelectedRef.current = false;
    setSelectedNodeId(undefined);
  }, [clearMatchNavTimer, setSelectedNodeId]);

  return {
    searchQuery,
    matchedNodeIds,
    safeMatchIndex,
    isCurrentMatchSelected,
    effectiveSettledNodeId,
    handleSearchChange,
    handleClearSearch,
    handleNextMatch,
    handlePrevMatch,
    stepToMatch,
    cancelPendingMatchNav,
    resetSearchTracking,
  };
}
