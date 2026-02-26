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

import { useVirtualizer } from '@tanstack/react-virtual';
import { render, screen } from '@testing-library/react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { AggregationLevel } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestAggregation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_aggregation.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import * as hooks from './hooks';
import { TestAggregationViewer } from './test_aggregation_viewer';

// Mock @tanstack/react-virtual must be before import
jest.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: jest.fn(),
}));
jest.mock('./hooks', () => ({
  useNodeAggregationsQuery: jest.fn(),
  useTestVerdictsQuery: jest.fn(),
  useSchemesQuery: jest.fn(),
  useAncestryAggregationsQueries: jest.fn(() => []),
}));

describe('TestAggregationViewer', () => {
  const mockScrollToIndex = jest.fn();

  beforeEach(() => {
    jest.clearAllMocks();
    (hooks.useAncestryAggregationsQueries as jest.Mock).mockReturnValue([]);

    // Default match for useVirtualizer
    (useVirtualizer as jest.Mock).mockImplementation(({ count }) => ({
      getVirtualItems: () =>
        Array.from({ length: count }).map((_, i) => ({
          index: i,
          start: i * 20,
          size: 20,
          measureElement: jest.fn(),
        })),
      getTotalSize: () => count * 20,
      scrollToIndex: mockScrollToIndex,
    }));
  });

  it('renders loading state when tree data is empty and loading', () => {
    (hooks.useNodeAggregationsQuery as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });
    // Verdicts hook (Skeleton)
    (hooks.useTestVerdictsQuery as jest.Mock).mockReturnValue({
      data: undefined,
      isLoading: true,
    });
    (hooks.useSchemesQuery as jest.Mock).mockReturnValue({ data: undefined });

    const invocation = { name: 'invocations/test' } as Invocation;
    const testVariant = {} as OutputTestVerdict;

    render(
      <FakeContextProvider>
        <TestAggregationViewer
          invocation={invocation}
          testVariant={testVariant}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument();
  });

  it('renders no failures message when tree data is empty and loaded', () => {
    (hooks.useNodeAggregationsQuery as jest.Mock).mockReturnValue({
      data: { aggregations: [] },
      isLoading: false,
    });
    (hooks.useTestVerdictsQuery as jest.Mock).mockReturnValue({
      data: { testVerdicts: [] },
      isLoading: false,
    });
    (hooks.useSchemesQuery as jest.Mock).mockReturnValue({ data: undefined });

    const invocation = { name: 'invocations/test' } as Invocation;
    const testVariant = {} as OutputTestVerdict;

    render(
      <FakeContextProvider>
        <TestAggregationViewer
          invocation={invocation}
          testVariant={testVariant}
        />
      </FakeContextProvider>,
    );

    expect(screen.getByText('No failures found.')).toBeInTheDocument();
  });

  it('renders aggregations when data is available', () => {
    (hooks.useNodeAggregationsQuery as jest.Mock).mockReturnValue({
      data: {
        aggregations: [
          TestAggregation.fromPartial({
            id: {
              level: AggregationLevel.MODULE,
              id: {
                moduleName: 'test_module',
                moduleScheme: 'test_scheme',
                moduleVariant: {
                  def: {
                    key: 'val',
                  },
                },
              },
            },
            verdictCounts: {
              failed: 5,
            },
          }),
        ],
      },
      isLoading: false,
    });
    (hooks.useTestVerdictsQuery as jest.Mock).mockReturnValue({
      data: {
        testVerdicts: [
          {
            testId: 'test_id_1',
            testIdStructured: {
              moduleName: 'test_module',
              moduleScheme: 'test_scheme',
              moduleVariant: {
                def: {
                  key: 'val',
                },
              },
              coarseName: '',
              fineName: '',
              caseName: 'case_1',
            },
            status: 5, // FAILED
          },
        ],
      },
      isLoading: false,
    });
    (hooks.useSchemesQuery as jest.Mock).mockReturnValue({ data: undefined });

    const invocation = { name: 'invocations/test' } as Invocation;
    const testVariant = {} as OutputTestVerdict;

    render(
      <FakeContextProvider>
        <TestAggregationViewer
          invocation={invocation}
          testVariant={testVariant}
        />
      </FakeContextProvider>,
    );
    expect(screen.getByText('Module:')).toBeInTheDocument();
    expect(screen.getByText('test_module')).toBeInTheDocument();
    expect(screen.getByText('(key=val)')).toBeInTheDocument();
    expect(screen.getByText(/5 failed/i)).toBeInTheDocument();
  });
});
