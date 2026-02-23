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

import { render, screen, fireEvent } from '@testing-library/react';

import { TestAggregationContext } from './context';
import { TestAggregationToolbar } from './test_aggregation_toolbar';

// Mock Aip160Autocomplete since it handles its own internal complexities
jest.mock('@/common/components/aip_160_autocomplete', () => ({
  Aip160Autocomplete: ({
    value,
    onValueCommit,
  }: {
    value: string;
    onValueCommit: (val: string) => void;
  }) => (
    <input
      data-testid="mock-aip-autocomplete"
      value={value}
      onChange={(e) => onValueCommit(e.target.value)}
    />
  ),
}));

describe('TestAggregationToolbar', () => {
  const mockSetSelectedStatuses = jest.fn();
  const mockSetAipFilter = jest.fn();
  const mockTriggerLoadMore = jest.fn();
  const mockOnLocateCurrentTest = jest.fn();

  const defaultContext = {
    selectedStatuses: new Set<string>(['Failed']),
    setSelectedStatuses: mockSetSelectedStatuses,
    aipFilter: '',
    setAipFilter: mockSetAipFilter,
    loadMoreTrigger: 0,
    triggerLoadMore: mockTriggerLoadMore,
    loadedCount: 100,
    setLoadedCount: jest.fn(),
    isLoadingMore: false,
    setIsLoadingMore: jest.fn(),
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  const setup = (contextOverrides = {}, propsOverrides = {}) => {
    return render(
      <TestAggregationContext.Provider
        value={{ ...defaultContext, ...contextOverrides }}
      >
        <TestAggregationToolbar
          onLocateCurrentTest={mockOnLocateCurrentTest}
          {...propsOverrides}
        />
      </TestAggregationContext.Provider>,
    );
  };

  it('renders components properly', () => {
    setup();

    // Toolbar elements
    expect(screen.getByTestId('mock-aip-autocomplete')).toBeInTheDocument();
    expect(screen.getByText(/Failed/i)).toBeInTheDocument(); // Chip category dropdown currently holds "Failed"
    expect(screen.getByText(/Loaded 100 tests/i)).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /load more/i }),
    ).toBeInTheDocument();

    // Locate button should be enabled
    expect(
      screen.getByRole('button', { name: /Locate current test/i }),
    ).toBeEnabled();
  });

  it('does not render locate button when onLocateCurrentTest is not provided', () => {
    setup({}, { onLocateCurrentTest: undefined });
    // The Locate button should not be rendered
    const locateBtn = screen.queryByRole('button', {
      name: /Locate current test/i,
    });
    expect(locateBtn).not.toBeInTheDocument();
  });

  it('updates aip filter when typing', async () => {
    setup();
    const input = screen.getByTestId('mock-aip-autocomplete');

    await fireEvent.change(input, { target: { value: 'status:FAILED' } });

    expect(mockSetAipFilter).toHaveBeenLastCalledWith('status:FAILED');
  });

  it('calls triggerLoadMore when load more is clicked', async () => {
    setup();
    const loadMoreBtn = screen.getByRole('button', {
      name: /load more/i,
    });

    await fireEvent.click(loadMoreBtn);

    expect(mockTriggerLoadMore).toHaveBeenCalledTimes(1);
  });
});
