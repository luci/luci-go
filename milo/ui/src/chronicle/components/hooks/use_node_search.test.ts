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
    // Before debounce elapses, auto-selection has not fired yet.
    expect(result.current.selectedNodeId).toBeUndefined();

    act(() => {
      jest.advanceTimersByTime(350);
    });

    // Automatically selects the first match after query debounce.
    expect(result.current.selectedNodeId).toBe('check-alpha');
    expect(result.current.safeMatchIndex).toBe(0);
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

    expect(workerInstance).not.toBeNull();
    expect(workerInstance!.postMessage).toHaveBeenCalledWith({
      nodes: SAMPLE_NODES,
      valueDataMap: undefined,
    });

    // User types a query and debounce elapses before the background worker finishes.
    act(() => {
      result.current.handleSearchChange('gamma');
    });
    act(() => {
      jest.advanceTimersByTime(350);
    });

    expect(result.current.matchedNodeIds).toEqual([]);
    expect(result.current.selectedNodeId).toBeUndefined();

    // Background worker finishes and posts the precomputed searchIndexMap.
    act(() => {
      workerInstance!.onmessage?.({
        data: {
          searchIndexMap: buildSearchIndexMap(SAMPLE_NODES),
        },
      });
    });

    expect(workerInstance!.terminate).toHaveBeenCalled();
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
});
