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

import { UseQueryResult } from '@tanstack/react-query';
import '@testing-library/jest-dom';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { useParams } from 'react-router';

import * as filterApiHooks from '@/crystal_ball/hooks/use_measurement_filter_api';
import {
  ListMeasurementFilterColumnsResponse,
  MeasurementFilterColumn_FilterScope,
  PerfChartSeries,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  SuggestMeasurementFilterValuesResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import {
  MAX_SPLIT_SUGGEST_RESULTS,
  SplitSeriesDialog,
} from './split_series_dialog';

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: jest.fn(),
}));

const mockedUseParams = jest.mocked(useParams);

jest.mock('@/crystal_ball/hooks/use_measurement_filter_api');
const mockedListColumns = jest.mocked(
  filterApiHooks.useListMeasurementFilterColumns,
);
const mockedSuggestValues = jest.mocked(
  filterApiHooks.useSuggestMeasurementFilterValues,
);

describe('SplitSeriesDialog', () => {
  const mockOnClose = jest.fn();
  const mockOnSplit = jest.fn();
  const sampleSeries: PerfChartSeries = PerfChartSeries.fromPartial({
    id: 'series-1',
    displayName: 'Test Series',
    filters: [],
  });

  beforeEach(() => {
    jest.clearAllMocks();
    mockedUseParams.mockReturnValue({ dashboardId: 'test-dashboard' });

    mockedListColumns.mockReturnValue({
      data: {
        measurementFilterColumns: [
          {
            column: 'build_branch',
            dataType: 1,
            sampleValues: [],
            primary: true,
            isMetricKey: false,
            applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
          },
          {
            column: 'atp_test_name',
            dataType: 1,
            sampleValues: [],
            primary: true,
            isMetricKey: false,
            applicableScopes: [MeasurementFilterColumn_FilterScope.WIDGET],
          },
        ],
        nextPageToken: '',
      },
      isLoading: false,
    } as unknown as UseQueryResult<ListMeasurementFilterColumnsResponse>);

    mockedSuggestValues.mockReturnValue({
      data: {
        values: ['value1', 'value2'],
        suggestions: [
          { value: 'value1', count: '1' },
          { value: 'value2', count: '1' },
        ],
      },
      isLoading: false,
    } as unknown as UseQueryResult<SuggestMeasurementFilterValuesResponse>);
  });

  it('renders correctly when open', () => {
    render(
      <SplitSeriesDialog
        open={true}
        onClose={mockOnClose}
        series={sampleSeries}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
      />,
    );

    expect(
      screen.getByRole('heading', { name: 'Split Series' }),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText('Select Dimension to Split On'),
    ).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: 'Split Series' }),
    ).toBeInTheDocument();
  });

  it('does not render when closed', () => {
    render(
      <SplitSeriesDialog
        open={false}
        onClose={mockOnClose}
        series={sampleSeries}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
      />,
    );

    expect(
      screen.queryByRole('heading', { name: 'Split Series' }),
    ).not.toBeInTheDocument();
  });

  it('calls onClose when Cancel is clicked', () => {
    render(
      <SplitSeriesDialog
        open={true}
        onClose={mockOnClose}
        series={sampleSeries}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
      />,
    );

    fireEvent.click(screen.getByText('Cancel'));
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('fetches values when column is selected', async () => {
    const globalFilters: PerfFilter[] = [
      {
        id: 'g1',
        column: 'build_branch',
        displayName: 'Global 1',
        dataSpecId: 'test-spec',
        textInput: {
          defaultValue: {
            values: ['main'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const widgetFilters: PerfFilter[] = [
      {
        id: 'w1',
        column: 'builder',
        displayName: 'Widget 1',
        dataSpecId: 'test-spec',
        textInput: {
          defaultValue: {
            values: ['linux-rel'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const seriesFilters: PerfFilter[] = [
      {
        id: 's1',
        column: 'build_target',
        displayName: 'Series 1',
        dataSpecId: 'test-spec',
        textInput: {
          defaultValue: {
            values: ['chrome'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const series = PerfChartSeries.fromPartial({
      ...sampleSeries,
      filters: seriesFilters,
    });

    render(
      <SplitSeriesDialog
        open={true}
        onClose={mockOnClose}
        series={series}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
        globalFilters={globalFilters}
        widgetFilters={widgetFilters}
      />,
    );

    // Select column
    fireEvent.mouseDown(screen.getByLabelText('Select Dimension to Split On'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'BUILD BRANCH' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'BUILD BRANCH' }));

    // Wait for values input to appear
    await waitFor(() => {
      expect(screen.getByLabelText('Select Values')).toBeInTheDocument();
    });

    expect(mockedSuggestValues).toHaveBeenCalledWith(
      expect.objectContaining({
        column: 'build_branch',
        maxResultCount: MAX_SPLIT_SUGGEST_RESULTS,
        skipCache: true,
        filter:
          'build_branch = "main" AND builder = "linux-rel" AND build_target = "chrome"',
      }),
      expect.any(Object),
    );
  });

  it('calls onSplit with selected values when Split is clicked', async () => {
    render(
      <SplitSeriesDialog
        open={true}
        onClose={mockOnClose}
        series={sampleSeries}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
      />,
    );

    // Select column
    fireEvent.mouseDown(screen.getByLabelText('Select Dimension to Split On'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'BUILD BRANCH' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'BUILD BRANCH' }));

    // Wait for values input to appear
    await waitFor(() => {
      expect(screen.getByLabelText('Select Values')).toBeInTheDocument();
    });

    // Select values
    fireEvent.mouseDown(screen.getByLabelText('Select Values'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'value1' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'value1' }));

    // Click Split
    fireEvent.click(screen.getByRole('button', { name: 'Split Series' }));

    expect(mockOnSplit).toHaveBeenCalledWith(['value1'], 'build_branch');
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });

  it('renders autocomplete dropdowns with a higher z-index than the drawer', async () => {
    render(
      <SplitSeriesDialog
        open={true}
        onClose={mockOnClose}
        series={sampleSeries}
        onSplit={mockOnSplit}
        dataSpecId="test-spec"
      />,
    );

    const drawer = screen.getByRole('dialog');
    const drawerStyle = window.getComputedStyle(drawer);
    const drawerZIndex = parseInt(drawerStyle.zIndex, 10);

    // 1. Check dimension chooser autocomplete popper z-index
    fireEvent.mouseDown(screen.getByLabelText('Select Dimension to Split On'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'BUILD BRANCH' }),
      ).toBeInTheDocument();
    });

    const optionElement1 = screen.getByRole('option', { name: 'BUILD BRANCH' });
    const popper1 = optionElement1.closest('.MuiAutocomplete-popper');
    expect(popper1).not.toBeNull();

    const popperStyle1 = window.getComputedStyle(popper1!);
    const popperZIndex1 = parseInt(popperStyle1.zIndex, 10);
    expect(popperZIndex1).toBeGreaterThan(drawerZIndex);

    // Select the option to reveal the second autocomplete
    fireEvent.click(optionElement1);

    // 2. Check value selector autocomplete popper z-index
    await waitFor(() => {
      expect(screen.getByLabelText('Select Values')).toBeInTheDocument();
    });

    fireEvent.mouseDown(screen.getByLabelText('Select Values'));
    await waitFor(() => {
      expect(
        screen.getByRole('option', { name: 'value1' }),
      ).toBeInTheDocument();
    });

    const optionElement2 = screen.getByRole('option', { name: 'value1' });
    const popper2 = optionElement2.closest('.MuiAutocomplete-popper');
    expect(popper2).not.toBeNull();

    const popperStyle2 = window.getComputedStyle(popper2!);
    const popperZIndex2 = parseInt(popperStyle2.zIndex, 10);
    expect(popperZIndex2).toBeGreaterThan(drawerZIndex);
  });
});
