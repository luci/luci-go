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

import { act, renderHook } from '@testing-library/react';
import { useState } from 'react';

import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';

import {
  buildSearchIndexMap,
  SearchIndexWorkerRequest,
} from '../../utils/search_index_worker';

import {
  buildMatchIndexMap,
  findNextMatchIndex,
  getNodeMatchIndex,
  SearchableNode,
  UseNodeSearchOptions,
  useNodeSearch,
} from './use_node_search';

jest.mock(
  '../../utils/search_index_worker?worker&url',
  () => 'mock-search-worker-url',
  {
    virtual: true,
  },
);

const SAMPLE_NODES: SearchableNode[] = [
  { id: 'check-alpha', label: 'Build Alpha' },
  { id: 'check-beta', label: 'Build Beta' },
  { id: 'check-gamma', label: 'Build Gamma' },
  { id: 'stage-deploy', label: 'Deploy Production' },
];

function useTestNodeSearch(
  overrides: Partial<UseNodeSearchOptions> = {},
  initialSelectedNodeId?: string,
) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>(
    initialSelectedNodeId,
  );
  const search = useNodeSearch({
    nodes: SAMPLE_NODES,
    selectedNodeId,
    setSelectedNodeId,
    ...overrides,
  });
  return {
    ...search,
    selectedNodeId,
    setSelectedNodeId,
  };
}

describe('useNodeSearch', () => {
  const originalWorker = global.Worker;

  beforeEach(() => {
    jest.useFakeTimers();

    class DefaultMockSearchWorker {
      onmessage: ((e: { data: unknown }) => void) | null = null;
      postMessage(data: SearchIndexWorkerRequest) {
        this.onmessage?.({
          data: {
            searchIndexMap: buildSearchIndexMap(data.nodes, data.valueDataMap),
          },
        });
      }
      terminate() {}
    }

    global.Worker = DefaultMockSearchWorker as unknown as typeof Worker;
  });

  afterEach(() => {
    global.Worker = originalWorker;
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  it('returns empty matches when searchQuery is empty and filters case-insensitively', () => {
    const { result } = renderHook(() => useTestNodeSearch());

    expect(result.current.searchQuery).toBe('');
    expect(result.current.matchedNodeIds).toEqual([]);
    expect(result.current.safeMatchIndex).toBe(0);

    act(() => {
      result.current.handleSearchChange('BUILD');
    });

    expect(result.current.searchQuery).toBe('BUILD');
    expect(result.current.matchedNodeIds).toEqual([
      'check-alpha',
      'check-beta',
      'check-gamma',
    ]);
    // When nothing is selected, typing text that matches at least one node immediately selects the first result.
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.safeMatchIndex).toBe(0);
    expect(result.current.isCurrentMatchSelected).toBe(true);

    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.safeMatchIndex).toBe(0);
    expect(result.current.isCurrentMatchSelected).toBe(true);
  });

  it('preserves existing selection on search if the selected node already matches', () => {
    const { result } = renderHook(() => useTestNodeSearch({}, 'check-beta'));

    act(() => {
      result.current.handleSearchChange('build');
    });

    expect(result.current.selectedNodeId).toBe('check-beta');
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.isCurrentMatchSelected).toBe(true);

    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.selectedNodeId).toBe('check-beta');
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.isCurrentMatchSelected).toBe(true);
  });

  it('iterates forward and backward through matches with wrap-around', () => {
    const { result } = renderHook(() => useTestNodeSearch());

    act(() => {
      result.current.handleSearchChange('build');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.safeMatchIndex).toBe(0);

    // Step forward: 0 -> 1 -> 2 -> 0 (wrap around)
    act(() => {
      result.current.handleNextMatch();
    });
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.selectedNodeId).toBe('check-beta');

    act(() => {
      result.current.handleNextMatch();
    });
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.selectedNodeId).toBe('check-gamma');

    act(() => {
      result.current.handleNextMatch();
    });
    expect(result.current.safeMatchIndex).toBe(0);
    expect(result.current.selectedNodeId).toBe('check-alpha');

    // Step backward: 0 -> 2 (wrap around) -> 1
    act(() => {
      result.current.handlePrevMatch();
    });
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.selectedNodeId).toBe('check-gamma');

    act(() => {
      result.current.handlePrevMatch();
    });
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.selectedNodeId).toBe('check-beta');
  });

  it('increments on every synchronous rapid click while freezing effectiveSettledNodeId until settled', () => {
    const onSettledMatch = jest.fn();
    const { result } = renderHook(() =>
      useTestNodeSearch({ onSettledMatch, settleDebounceMs: 300 }),
    );

    act(() => {
      result.current.handleSearchChange('build');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.effectiveSettledNodeId).toBe('check-alpha');
    expect(onSettledMatch).toHaveBeenCalledTimes(1);
    expect(onSettledMatch).toHaveBeenCalledWith('check-alpha');
    onSettledMatch.mockClear();

    // Fire two rapid synchronous clicks within the same act() block:
    // 0 ('check-alpha') -> 1 ('check-beta') -> 2 ('check-gamma')
    act(() => {
      result.current.handleNextMatch();
      result.current.handleNextMatch();
    });

    // Urgent index and selection immediately reflect the second click ('check-gamma'),
    // while effectiveSettledNodeId stays frozen at 'check-alpha' during navigation.
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.selectedNodeId).toBe('check-gamma');
    expect(result.current.effectiveSettledNodeId).toBe('check-alpha');
    expect(onSettledMatch).not.toHaveBeenCalled();

    // Advance 200ms (less than 300ms settleDebounceMs) and click once more:
    // 2 ('check-gamma') -> 0 ('check-alpha') -> 1 ('check-beta')
    act(() => {
      jest.advanceTimersByTime(200);
      result.current.handleNextMatch();
      result.current.handleNextMatch();
    });

    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.selectedNodeId).toBe('check-beta');
    expect(result.current.effectiveSettledNodeId).toBe('check-alpha');
    expect(onSettledMatch).not.toHaveBeenCalled();

    // Advance full 300ms settle window: effectiveSettledNodeId and onSettledMatch commit 'check-beta'.
    act(() => {
      jest.advanceTimersByTime(300);
    });

    expect(result.current.effectiveSettledNodeId).toBe('check-beta');
    expect(onSettledMatch).toHaveBeenCalledTimes(1);
    expect(onSettledMatch).toHaveBeenCalledWith('check-beta');
  });

  it('marks isCurrentMatchSelected false when a non-matching node is selected and returns to the stored match on Next or Prev', () => {
    const { result } = renderHook(() => useTestNodeSearch());

    act(() => {
      result.current.handleSearchChange('build');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });

    // Step to index 1 ('check-beta') and let it settle.
    act(() => {
      result.current.handleNextMatch();
      jest.advanceTimersByTime(350);
    });
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.isCurrentMatchSelected).toBe(true);
    expect(result.current.selectedNodeId).toBe('check-beta');

    // User clicks a node on the canvas that is not in matchedNodeIds ('stage-deploy').
    act(() => {
      result.current.setSelectedNodeId('stage-deploy');
    });

    // Stored index stays at 1, but isCurrentMatchSelected is now false ("Match ? of Y").
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.isCurrentMatchSelected).toBe(false);

    // Pressing Next returns to the previously-selected match (index 1, 'check-beta').
    act(() => {
      result.current.handleNextMatch();
    });
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.isCurrentMatchSelected).toBe(true);
    expect(result.current.selectedNodeId).toBe('check-beta');

    // Pressing Next again now advances from 1 -> 2 ('check-gamma').
    act(() => {
      result.current.handleNextMatch();
      jest.advanceTimersByTime(350);
    });
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.isCurrentMatchSelected).toBe(true);
    expect(result.current.selectedNodeId).toBe('check-gamma');

    // Select non-matching node again: stored index is 2, isCurrentMatchSelected is false.
    act(() => {
      result.current.setSelectedNodeId('stage-deploy');
    });
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.isCurrentMatchSelected).toBe(false);

    // Pressing Prev also returns to the previously-selected match (index 2, 'check-gamma').
    act(() => {
      result.current.handlePrevMatch();
    });
    expect(result.current.safeMatchIndex).toBe(2);
    expect(result.current.isCurrentMatchSelected).toBe(true);
    expect(result.current.selectedNodeId).toBe('check-gamma');

    // Pressing Prev again now steps backward from 2 -> 1 ('check-beta').
    act(() => {
      result.current.handlePrevMatch();
    });
    expect(result.current.safeMatchIndex).toBe(1);
    expect(result.current.selectedNodeId).toBe('check-beta');
  });

  it('clears search query, cancels pending navigation, and resets selection on handleClearSearch', () => {
    const onSettledMatch = jest.fn();
    const { result } = renderHook(() =>
      useTestNodeSearch({ onSettledMatch, settleDebounceMs: 300 }),
    );

    act(() => {
      result.current.handleSearchChange('build');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });
    expect(onSettledMatch).toHaveBeenCalledWith('check-alpha');
    onSettledMatch.mockClear();

    act(() => {
      result.current.handleNextMatch();
      result.current.handleClearSearch();
    });

    expect(result.current.searchQuery).toBe('');
    expect(result.current.matchedNodeIds).toEqual([]);
    expect(result.current.safeMatchIndex).toBe(0);
    expect(result.current.selectedNodeId).toBeUndefined();

    // Ensure the canceled timer does not fire onSettledMatch later.
    act(() => {
      jest.advanceTimersByTime(500);
    });
    expect(onSettledMatch).not.toHaveBeenCalled();
  });

  it('indexes externalized ValueData payloads from valueDataMap', () => {
    const nodesWithValueRef: SearchableNode[] = [
      ...SAMPLE_NODES,
      {
        id: 'check-with-payload',
        label: 'Test Check',
        view: Check.fromPartial({
          identifier: { id: 'check-with-payload' },
          results: [{ data: [{ digest: 'digest-res-1' }] }],
        }),
      },
    ];
    const valueDataMap = new Map<string, ValueData>([
      [
        'digest-res-1',
        ValueData.fromPartial({
          json: {
            value: JSON.stringify({ testSuite: 'CtsWidgetTestCases' }),
          },
        }),
      ],
    ]);

    const { result } = renderHook(() =>
      useTestNodeSearch({ nodes: nodesWithValueRef, valueDataMap }),
    );

    act(() => {
      result.current.handleSearchChange('CtsWidgetTestCases');
    });

    expect(result.current.matchedNodeIds).toEqual(['check-with-payload']);
  });

  it('waits for the background worker to finish before resolving matches and auto-selecting', () => {
    let workerInstance: {
      onmessage: ((e: { data: unknown }) => void) | null;
      postMessage: jest.Mock;
      terminate: jest.Mock;
    } | null = null;

    class DeferredMockSearchWorker {
      onmessage: ((e: { data: unknown }) => void) | null = null;
      postMessage = jest.fn();
      terminate = jest.fn();
      constructor() {
        // eslint-disable-next-line @typescript-eslint/no-this-alias
        workerInstance = this;
      }
    }

    global.Worker = DeferredMockSearchWorker as unknown as typeof Worker;

    const { result } = renderHook(() => useTestNodeSearch());

    expect(result.current.isIndexReady).toBe(false);
    expect(workerInstance).not.toBeNull();
    expect(workerInstance!.postMessage).toHaveBeenCalledWith({
      nodes: SAMPLE_NODES,
      valueDataMap: undefined,
    });

    // Background worker finishes and posts the precomputed searchIndexMap.
    act(() => {
      workerInstance!.onmessage?.({
        data: {
          searchIndexMap: buildSearchIndexMap(SAMPLE_NODES),
        },
      });
    });

    expect(workerInstance!.terminate).toHaveBeenCalled();
    expect(result.current.isIndexReady).toBe(true);

    // User types a query once the background worker finishes and input enables.
    act(() => {
      result.current.handleSearchChange('gamma');
    });

    expect(result.current.matchedNodeIds).toEqual(['check-gamma']);
    expect(result.current.selectedNodeId).toBe('check-gamma');
  });

  it('preserves normalized query and resets tracking state when resetSearchTracking is called', () => {
    const onSettledMatch = jest.fn();
    const { result } = renderHook(() => useTestNodeSearch({ onSettledMatch }));

    act(() => {
      result.current.handleSearchChange('alpha');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.matchedNodeIds).toEqual(['check-alpha']);
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(onSettledMatch).toHaveBeenCalledWith('check-alpha');
    onSettledMatch.mockClear();

    act(() => {
      result.current.resetSearchTracking();
    });

    // Search query remains 'alpha' while match tracking is reset.
    expect(result.current.searchQuery).toBe('alpha');
    expect(result.current.safeMatchIndex).toBe(0);
    expect(result.current.isCurrentMatchSelected).toBe(true);
  });

  it('re-evaluates auto-selection and invokes onSettledMatch when searchable nodes change with an active query', () => {
    const onSettledMatch = jest.fn();
    let currentNodes = SAMPLE_NODES;
    const { result, rerender } = renderHook(() =>
      useTestNodeSearch({ nodes: currentNodes, onSettledMatch }),
    );

    act(() => {
      result.current.handleSearchChange('build');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.matchedNodeIds).toEqual([
      'check-alpha',
      'check-beta',
      'check-gamma',
    ]);
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(onSettledMatch).toHaveBeenCalledWith('check-alpha');
    onSettledMatch.mockClear();

    // Now update nodes to a filtered set (e.g. only gamma and deploy)
    currentNodes = [
      { id: 'check-gamma', label: 'Build Gamma' },
      { id: 'stage-deploy', label: 'Deploy Production' },
    ];
    rerender();

    expect(result.current.matchedNodeIds).toEqual(['check-gamma']);
    expect(result.current.selectedNodeId).toBe('check-gamma');
    expect(onSettledMatch).toHaveBeenCalledWith('check-gamma');
  });

  it('does not hijack a manual non-matching selection when new nodes are retrieved from the backend', () => {
    let currentNodes = SAMPLE_NODES;
    const { result, rerender } = renderHook(() =>
      useTestNodeSearch({ nodes: currentNodes }),
    );

    // 1. Search 'build' -> check-alpha selected
    act(() => {
      result.current.handleSearchChange('build');
    });
    expect(result.current.selectedNodeId).toBe('check-alpha');

    // 2. User manually selects non-matching node 'stage-deploy'
    act(() => {
      result.current.setSelectedNodeId('stage-deploy');
    });
    expect(result.current.selectedNodeId).toBe('stage-deploy');
    expect(result.current.isCurrentMatchSelected).toBe(false);

    // 3. Updated workplan arrives (nodes update)
    currentNodes = [
      ...SAMPLE_NODES,
      { id: 'check-delta', label: 'Build Delta' },
    ];
    rerender();

    // Manual selection must NOT be hijacked
    expect(result.current.selectedNodeId).toBe('stage-deploy');
    expect(result.current.isCurrentMatchSelected).toBe(false);

    // 4. Stepping to next match returns to the search match
    act(() => {
      result.current.handleNextMatch();
    });
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.isCurrentMatchSelected).toBe(true);
  });

  describe('tracking selection via search results across keystrokes and manual selection', () => {
    it('tracks whether current selection was selected because of search results through all state transitions', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // 1) when nothing is selected and user types text that matches at least one node:
      // selects first result
      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.isCurrentMatchSelected).toBe(true);
      expect(result.current.safeMatchIndex).toBe(0);

      // If currently selected item was selected because of search results and remains a result after key is pressed,
      // it should remain selected.
      act(() => {
        result.current.handleSearchChange('build a');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.isCurrentMatchSelected).toBe(true);
      expect(result.current.safeMatchIndex).toBe(0);

      // If it was selected because of the search results and is no longer a result, we should select the first result.
      act(() => {
        result.current.handleSearchChange('build b');
      });
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);
      expect(result.current.safeMatchIndex).toBe(0);

      // User manually selects a node that is NOT a search result ('stage-deploy' for 'build b').
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // If the currently selected item was not selected because of the search results and still is not a result,
      // it should remain selected and remain not selected because of the search results, with "? of X" displayed.
      act(() => {
        result.current.handleSearchChange('build beta');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);
      expect(result.current.matchedNodeIds).toEqual(['check-beta']);

      // If the currently selected item was not selected because of the search results and but is now a result,
      // it should remain selected and should now be considered selected because of the search results.
      act(() => {
        result.current.handleSearchChange('deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(true);
      expect(result.current.safeMatchIndex).toBe(0);

      // User clears search by emptying input
      act(() => {
        result.current.handleSearchChange('');
      });
      expect(result.current.selectedNodeId).toBeUndefined();
      expect(result.current.isCurrentMatchSelected).toBe(false);
    });

    it('considers selection via search when user clicks next/prev buttons or manually selects a search result', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // 2) when the user uses the next/prev buttons in the search results panel
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      act(() => {
        result.current.handlePrevMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // 3) when the user manually selects a node that is a search result
      act(() => {
        result.current.setSelectedNodeId('check-gamma');
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');
      expect(result.current.isCurrentMatchSelected).toBe(true);
      expect(result.current.safeMatchIndex).toBe(2);

      // Deselecting by clicking canvas background sets selection to undefined
      act(() => {
        result.current.setSelectedNodeId(undefined);
      });
      expect(result.current.selectedNodeId).toBeUndefined();
      expect(result.current.isCurrentMatchSelected).toBe(false);
      // "? of 3"
      expect(result.current.safeMatchIndex).toBe(2);

      // Pressing Next returns to stored match (check-gamma) and re-enables search selection
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });

    it('clears selection when an item selected via search becomes non-matching and there are 0 matches', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      act(() => {
        result.current.handleSearchChange('gamma');
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      act(() => {
        result.current.handleSearchChange('gammaxyz');
      });
      expect(result.current.matchedNodeIds).toEqual([]);
      expect(result.current.selectedNodeId).toBeUndefined();
      expect(result.current.isCurrentMatchSelected).toBe(false);
    });

    it('advances to the next match in graph order when the query changes and the selected match no longer matches', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');

      // Step to check-beta (index 1 of [check-alpha, check-beta, check-gamma])
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');

      // Now change search to 'gamma': check-beta no longer matches.
      // Next match in graph order after check-beta is check-gamma.
      act(() => {
        result.current.handleSearchChange('gamma');
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');
      expect(result.current.safeMatchIndex).toBe(0);
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // From check-gamma, change query to 'alpha': next match wraps around to check-alpha.
      act(() => {
        result.current.handleSearchChange('alpha');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.safeMatchIndex).toBe(0);
    });

    it('handles search terms changing while non-matching item is selected, preserving match memory if still matching', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // Search 'build' -> [check-alpha, check-beta, check-gamma]
      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');

      // Step to check-gamma (index 2)
      act(() => {
        result.current.handleNextMatch();
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');

      // User selects a non-matching item (stage-deploy)
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // Now user modifies search terms to 'build b':
      // stage-deploy is still non-matching, so selection remains stage-deploy.
      // But matched index is reset to 0 (pointing to check-beta).
      act(() => {
        result.current.handleSearchChange('build b');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.matchedNodeIds).toEqual(['check-beta']);
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // Pressing Next advances to the next matching node (check-beta)
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // Select stage-deploy again and change search to 'build'
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');

      // Pressing Prev from stage-deploy returns to the previously navigated match (check-beta)
      act(() => {
        result.current.handlePrevMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // Pressing Prev again steps to the previous match before check-beta (check-alpha)
      act(() => {
        result.current.handlePrevMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });

    it('returns to Match 0 symmetrically when clicking off-match canvas node while on Match 0', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // Search 'build' -> [check-alpha, check-beta, check-gamma]
      act(() => {
        result.current.handleSearchChange('build');
      });
      // Match 0 (check-alpha) is auto-selected
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.safeMatchIndex).toBe(0);
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // User clicks non-matching canvas node (stage-deploy)
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);
      expect(result.current.safeMatchIndex).toBe(0);

      // Pressing Next returns to Match 0 (check-alpha) rather than skipping ahead
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.safeMatchIndex).toBe(0);
      expect(result.current.isCurrentMatchSelected).toBe(true);

      // Click off-match node again and verify Prev also returns to Match 0
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');

      act(() => {
        result.current.handlePrevMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');
      expect(result.current.safeMatchIndex).toBe(0);
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });

    it('preserves the last-selected match when typing more after clicking canvas (no node selected)', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // 1. Type query 'build' -> [check-alpha, check-beta, check-gamma]
      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');

      // 2. Click Next -> steps to check-beta
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');

      // 3. Select canvas (no node selected)
      act(() => {
        result.current.setSelectedNodeId(undefined);
      });
      expect(result.current.selectedNodeId).toBeUndefined();
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // 4. Type some more (query refined to 'build b') where check-beta is still a valid match
      act(() => {
        result.current.handleSearchChange('build b');
      });
      // Should NOT switch to the first match if check-beta is still valid; it should select check-beta
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });

    it('advances to next match in graph order when typing more after clicking canvas if last-selected match no longer matches', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // Type 'build' -> [check-alpha, check-beta, check-gamma]
      act(() => {
        result.current.handleSearchChange('build');
      });
      // Step to check-beta
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');

      // Click canvas (no node selected)
      act(() => {
        result.current.setSelectedNodeId(undefined);
      });
      expect(result.current.selectedNodeId).toBeUndefined();

      // Type 'gamma': check-beta no longer matches. Next match in graph order after check-beta is check-gamma.
      act(() => {
        result.current.handleSearchChange('gamma');
      });
      expect(result.current.selectedNodeId).toBe('check-gamma');
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });

    it('returns to previously navigated match on Next after selecting non-matching node and typing more text', () => {
      const { result } = renderHook(() => useTestNodeSearch());

      // 1. Type query 'build' -> [check-alpha, check-beta, check-gamma]
      act(() => {
        result.current.handleSearchChange('build');
      });
      expect(result.current.selectedNodeId).toBe('check-alpha');

      // 2. Click Next -> steps to check-beta
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');

      // 3. Select non-matching node (stage-deploy) on canvas
      act(() => {
        result.current.setSelectedNodeId('stage-deploy');
      });
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // 4. Type some more ('build b') where check-beta is still a match
      act(() => {
        result.current.handleSearchChange('build b');
      });
      // Selection should remain on stage-deploy (manual selection not hijacked)
      expect(result.current.selectedNodeId).toBe('stage-deploy');
      expect(result.current.isCurrentMatchSelected).toBe(false);

      // 5. Click Next -> should return to check-beta (the previously navigated match)
      act(() => {
        result.current.handleNextMatch();
      });
      expect(result.current.selectedNodeId).toBe('check-beta');
      expect(result.current.isCurrentMatchSelected).toBe(true);
    });
  });

  describe('buildMatchIndexMap and getNodeMatchIndex', () => {
    it('maps direct IDs and base IDs without prefix to match indices', () => {
      const matchMap = buildMatchIndexMap(['Ccheck123:R1', 'Sstage456']);
      expect(matchMap.get('Ccheck123:R1')).toBe(0);
      expect(matchMap.get('check123')).toBe(0);
      expect(matchMap.get('Sstage456')).toBe(1);
      expect(matchMap.get('stage456')).toBe(1);
      expect(matchMap.get('nonexistent')).toBeUndefined();
    });

    it('preserves the earliest index for duplicates or base ID collisions', () => {
      const matchMap = buildMatchIndexMap(['Ccheck123:R1', 'check123']);
      expect(matchMap.get('Ccheck123:R1')).toBe(0);
      expect(matchMap.get('check123')).toBe(0);
    });

    it('finds node match index using either array or precomputed Map', () => {
      const matches = ['check123', 'stage456'];
      const matchMap = buildMatchIndexMap(matches);

      expect(getNodeMatchIndex('check123', matches)).toBe(0);
      expect(getNodeMatchIndex('Ccheck123:R1', matches)).toBe(0);
      expect(getNodeMatchIndex('stage456', matches)).toBe(1);
      expect(getNodeMatchIndex('Sstage456:A1', matches)).toBe(1);
      expect(getNodeMatchIndex('nonexistent', matches)).toBe(-1);

      expect(getNodeMatchIndex('check123', matchMap)).toBe(0);
      expect(getNodeMatchIndex('Ccheck123:R1', matchMap)).toBe(0);
      expect(getNodeMatchIndex('stage456', matchMap)).toBe(1);
      expect(getNodeMatchIndex('Sstage456:A1', matchMap)).toBe(1);
      expect(getNodeMatchIndex('nonexistent', matchMap)).toBe(-1);
    });
  });

  describe('findNextMatchIndex', () => {
    const testNodes: SearchableNode[] = [
      { id: 'node-0', label: 'Node 0' },
      { id: 'node-1', label: 'Node 1' },
      { id: 'node-2', label: 'Node 2' },
      { id: 'node-3', label: 'Node 3' },
      { id: 'node-4', label: 'Node 4' },
    ];
    const matches = ['node-1', 'node-3'];

    it('finds the next match after a selected node', () => {
      // Selected node-0 -> next match in nodes is node-1 (match index 0)
      expect(findNextMatchIndex('node-0', testNodes, matches)).toBe(0);
      // Selected node-1 -> next match in nodes is node-3 (match index 1)
      expect(findNextMatchIndex('node-1', testNodes, matches)).toBe(1);
      // Selected node-2 -> next match in nodes is node-3 (match index 1)
      expect(findNextMatchIndex('node-2', testNodes, matches)).toBe(1);
    });

    it('wraps around to the first match when at or past the last match', () => {
      // Selected node-3 -> next match wraps around to node-1 (match index 0)
      expect(findNextMatchIndex('node-3', testNodes, matches)).toBe(0);
      // Selected node-4 -> next match wraps around to node-1 (match index 0)
      expect(findNextMatchIndex('node-4', testNodes, matches)).toBe(0);
    });

    it('returns 0 when selectedId is undefined', () => {
      expect(findNextMatchIndex(undefined, testNodes, matches)).toBe(0);
    });

    it('handles empty matches gracefully', () => {
      expect(findNextMatchIndex('node-1', testNodes, [])).toBe(0);
    });
  });
});
