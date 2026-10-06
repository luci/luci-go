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
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import { useState } from 'react';

import { WorkPlan } from '@/proto/turboci/graph/orchestrator/v1/workplan.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { WorkflowType } from '../fake_turboci_graph';
import { ChronicleNode } from '../utils/graph_builder';
import {
  buildSearchIndexMap,
  SearchIndexWorkerRequest,
} from '../utils/search_index_worker';

import { ChronicleContext, ChronicleContextType } from './context';
import { Component as GraphView } from './graph_view';
import {
  MATCHED_NODE_STYLE,
  SELECTED_NODE_STYLE,
} from './hooks/use_graph_highlighting';

const mockFitView = jest.fn().mockResolvedValue(true);

jest.mock('@xyflow/react', () => {
  const actual = jest.requireActual('@xyflow/react');
  return {
    ...actual,
    useReactFlow: () => ({
      ...actual.useReactFlow(),
      fitView: mockFitView,
    }),
  };
});

jest.mock('../utils/graph_worker?worker&url', () => 'mock-worker-url', {
  virtual: true,
});

jest.mock(
  '../utils/search_index_worker?worker&url',
  () => 'mock-search-worker-url',
  {
    virtual: true,
  },
);

jest.mock('react-resizable-panels', () => ({
  PanelGroup: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Panel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  PanelResizeHandle: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

jest.mock('@/generic_libs/components/routed_tabs/context', () => ({
  ...jest.requireActual('@/generic_libs/components/routed_tabs/context'),
  useDeclareTabId: jest.fn(),
}));

const mockNodes: ChronicleNode[] = [
  {
    id: 'stage-alpha-1',
    position: { x: 100, y: 100 },
    data: {
      label: 'Alpha Stage 1',
      fullLabel: 'Alpha Stage 1',
    },
  },
  {
    id: 'stage-alpha-2',
    position: { x: 200, y: 100 },
    data: {
      label: 'Alpha Stage 2',
      fullLabel: 'Alpha Stage 2',
    },
  },
  {
    id: 'stage-alpha-3',
    position: { x: 300, y: 100 },
    data: {
      label: 'Alpha Stage 3',
      fullLabel: 'Alpha Stage 3',
    },
  },
  {
    id: 'check-beta-1',
    position: { x: 400, y: 100 },
    data: {
      label: 'Beta Check 1',
      fullLabel: 'Beta Check 1',
    },
  },
];

const mockGraph = WorkPlan.fromPartial({ stages: [], checks: [] });
const mockValueDataMap = new Map();

function TestWrapper() {
  const [selectedNodeId, setSelectedNodeId] = useState<string | undefined>(
    undefined,
  );

  const contextValue: ChronicleContextType = {
    workplanId: 'demo',
    graph: mockGraph,
    valueDataMap: mockValueDataMap,
    activeEnvironment: '',
    setActiveEnvironment: jest.fn(),
    workflowType: WorkflowType.ANDROID,
    setWorkflowType: jest.fn(),
    selectedNodeId,
    setSelectedNodeId,
    detecting: false,
    setDetecting: jest.fn(),
    detectionFailed: false,
    setDetectionFailed: jest.fn(),
    showEnvDialog: false,
    setShowEnvDialog: jest.fn(),
    foundEnvironments: [],
    requestedEnvFailed: undefined,
    failedEnvironments: [],
    detectionCancelled: false,
    setDetectionCancelled: jest.fn(),
  };

  return (
    <FakeContextProvider>
      <ChronicleContext.Provider value={contextValue}>
        <div data-testid="selected-node-id">{selectedNodeId || 'none'}</div>
        <GraphView />
      </ChronicleContext.Provider>
    </FakeContextProvider>
  );
}

describe('GraphView filter match navigation', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    mockFitView.mockClear();
    class MockWorker {
      onmessage: ((e: { data: unknown }) => void) | null = null;
      postMessage(data: unknown) {
        if (data && typeof data === 'object' && 'nodes' in data) {
          const req = data as SearchIndexWorkerRequest;
          this.onmessage?.({
            data: {
              searchIndexMap: buildSearchIndexMap(req.nodes, req.valueDataMap),
            },
          });
          return;
        }
        queueMicrotask(() => {
          if (this.onmessage) {
            this.onmessage({
              data: {
                nodes: mockNodes,
                edges: [],
              },
            });
          }
        });
      }
      terminate() {}
    }

    global.Worker = MockWorker as unknown as typeof Worker;
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('does not display match row when no search query is entered', async () => {
    render(<TestWrapper />);

    expect(
      screen.queryByTestId('search-match-display'),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Previous match' }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Next match' }),
    ).not.toBeInTheDocument();
  });

  it('iterates forward and backward through matches with wrap-around', async () => {
    render(<TestWrapper />);

    // Wait for worker layout to populate
    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    const matchDisplay = screen.getByTestId('search-match-display');
    expect(matchDisplay).toHaveTextContent('Match 1 of 3');

    const prevButton = screen.getByRole('button', { name: 'Previous match' });
    const nextButton = screen.getByRole('button', { name: 'Next match' });
    expect(prevButton).not.toBeDisabled();
    expect(nextButton).not.toBeDisabled();

    // Click next button: should advance to Match 2 of 3 and select stage-alpha-2
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );

    // Click next button: should advance to Match 3 of 3 and select stage-alpha-3
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 3 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-3',
    );

    // Click next button at end: should wrap around to Match 1 of 3 and select stage-alpha-1
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 1 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-1',
    );

    // Click previous button at start: should wrap around to Match 3 of 3 and select stage-alpha-3
    fireEvent.click(prevButton);
    expect(matchDisplay).toHaveTextContent('Match 3 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-3',
    );
  });

  it('displays Match 0 of 0 and disables buttons when search query has no matches', async () => {
    render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'nonexistent-query' } });

    const matchDisplay = screen.getByTestId('search-match-display');
    expect(matchDisplay).toHaveTextContent('Match 0 of 0');

    const prevButton = screen.getByRole('button', { name: 'Previous match' });
    const nextButton = screen.getByRole('button', { name: 'Next match' });
    expect(prevButton).toBeDisabled();
    expect(nextButton).toBeDisabled();
  });

  it('highlights search matches with yellow inner outline and composes blue selection border on selected match', async () => {
    const { container } = render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    const nextButton = screen.getByRole('button', { name: 'Next match' });
    // Select stage-alpha-2 via Next button
    fireEvent.click(nextButton);

    const node1 = container.querySelector('[data-id="stage-alpha-1"]');
    const node2 = container.querySelector('[data-id="stage-alpha-2"]');
    const node3 = container.querySelector('[data-id="stage-alpha-3"]');
    const nodeBeta = container.querySelector('[data-id="check-beta-1"]');

    // Advance past debounced match navigation settle window
    act(() => {
      jest.advanceTimersByTime(350);
    });
    expect(node2).toHaveStyle(`border-top: ${SELECTED_NODE_STYLE.borderTop}`);
    expect(node2).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    // Unselected search matches remain highlighted with yellow inner outline
    expect(node1).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    expect(node3).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    // Non-matching node has neither highlight
    expect(nodeBeta).not.toHaveStyle(
      `border-top: ${SELECTED_NODE_STYLE.borderTop}`,
    );
    expect(nodeBeta).not.toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
  });

  it('allows rapid clicking to jump between matches quickly and defers full highlighting until settled', async () => {
    const { container } = render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    const matchDisplay = screen.getByTestId('search-match-display');
    expect(matchDisplay).toHaveTextContent('Match 1 of 3');

    const nextButton = screen.getByRole('button', { name: 'Next match' });
    const node2 = container.querySelector('[data-id="stage-alpha-2"]');
    const node3 = container.querySelector('[data-id="stage-alpha-3"]');

    // Rapidly click 5 times in a row (click-click-click-click-click):
    // Match 1 -> 2 -> 3 -> 1 -> 2 -> 3
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 3 of 3');
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 1 of 3');
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 3 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-3',
    );

    // Intermediate match (node2) should never receive selection-blue border,
    // and final target (node3) receives selection-blue border once settled.
    expect(node2).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    expect(node2).not.toHaveStyle(
      `border-top: ${SELECTED_NODE_STYLE.borderTop}`,
    );
    act(() => {
      jest.advanceTimersByTime(350);
    });
    expect(node3).toHaveStyle(`border-top: ${SELECTED_NODE_STYLE.borderTop}`);
    expect(node3).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    expect(node2).toHaveStyle(`outline: ${MATCHED_NODE_STYLE.outline}`);
    expect(node2).not.toHaveStyle(
      `border-top: ${SELECTED_NODE_STYLE.borderTop}`,
    );
  });

  it('does not fit view when iterating between search results if Fit View on Selection is unchecked', async () => {
    const { container } = render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');
    // Wait for initial graph load fitView animation frame to flush
    await waitFor(() => {
      expect(mockFitView).toHaveBeenCalled();
    });

    // Uncheck "Fit View on Selection"
    const fitViewCheckbox = screen.getByRole('checkbox', {
      name: 'Fit View on Selection',
    });
    fireEvent.click(fitViewCheckbox);
    expect(fitViewCheckbox).not.toBeChecked();

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    mockFitView.mockClear();

    const nextButton = screen.getByRole('button', { name: 'Next match' });
    fireEvent.click(nextButton);

    expect(screen.getByTestId('search-match-display')).toHaveTextContent(
      'Match 2 of 3',
    );
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );
    // Advance fake timers past the debounce window and verify the settled highlight
    // has been applied without calling fitView while unchecked.
    act(() => {
      jest.advanceTimersByTime(350);
    });
    const node2 = container.querySelector('[data-id="stage-alpha-2"]');
    expect(node2).toHaveStyle(`border-top: ${SELECTED_NODE_STYLE.borderTop}`);
    expect(mockFitView).not.toHaveBeenCalled();

    // Re-checking "Fit View on Selection" should immediately fit view to the selected match
    fireEvent.click(fitViewCheckbox);
    expect(fitViewCheckbox).toBeChecked();
    await waitFor(() => {
      expect(mockFitView).toHaveBeenCalledWith(
        expect.objectContaining({
          nodes: [{ id: 'stage-alpha-2' }],
        }),
      );
    });
  });

  it('clears search query and selection when clicking the clear (X) button', async () => {
    render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    expect(
      screen.queryByRole('button', { name: 'Clear search' }),
    ).not.toBeInTheDocument();

    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    const clearButton = screen.getByRole('button', { name: 'Clear search' });
    expect(clearButton).toBeInTheDocument();

    fireEvent.click(clearButton);

    expect(searchInput).toHaveValue('');
    expect(
      screen.queryByTestId('search-match-display'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent('none');
    expect(searchInput).toHaveFocus();
  });

  it('displays Match ? of Y when a non-matching node is selected and returns to the previous match on Next or Prev', async () => {
    render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    const matchDisplay = screen.getByTestId('search-match-display');
    const nextButton = screen.getByRole('button', { name: 'Next match' });
    const prevButton = screen.getByRole('button', { name: 'Previous match' });

    // Step to Match 2 of 3 (stage-alpha-2)
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );

    // Click a node that is not a search result (Beta Check 1)
    fireEvent.click(screen.getByText('Beta Check 1'));
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'check-beta-1',
    );
    expect(matchDisplay).toHaveTextContent('Match ? of 3');

    // Pressing Next returns to the previously-selected match (Match 2 of 3, stage-alpha-2)
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );

    // Click the non-matching node again -> Match ? of 3
    fireEvent.click(screen.getByText('Beta Check 1'));
    expect(matchDisplay).toHaveTextContent('Match ? of 3');

    // Pressing Previous also returns to the previously-selected match (Match 2 of 3, stage-alpha-2)
    fireEvent.click(prevButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );

    // Clicking the currently selected match node (Alpha Stage 2) unselects it -> Match ? of 3
    fireEvent.click(screen.getByText('Alpha Stage 2'));
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent('none');
    expect(matchDisplay).toHaveTextContent('Match ? of 3');

    // Pressing Next returns to the previously-selected match (Match 2 of 3, stage-alpha-2)
    fireEvent.click(nextButton);
    expect(matchDisplay).toHaveTextContent('Match 2 of 3');
    expect(screen.getByTestId('selected-node-id')).toHaveTextContent(
      'stage-alpha-2',
    );
  });

  it('fits view properly when toggling Show Critical Path with an active search query', async () => {
    render(<TestWrapper />);

    await screen.findByText('Alpha Stage 1');
    mockFitView.mockClear();

    const searchInput = screen.getByPlaceholderText('Search nodes...');
    fireEvent.change(searchInput, { target: { value: 'Alpha' } });

    act(() => {
      jest.advanceTimersByTime(350);
    });
    expect(mockFitView).toHaveBeenCalledWith(
      expect.objectContaining({
        nodes: [{ id: 'stage-alpha-1' }],
      }),
    );

    mockFitView.mockClear();

    // Toggle Show Critical Path
    const criticalPathCheckbox = screen.getByLabelText('Show Critical Path');
    fireEvent.click(criticalPathCheckbox);

    // After worker delivers the new layout, fitView should be called and not stranded
    await waitFor(() => {
      expect(mockFitView).toHaveBeenCalled();
    });
  });
});
