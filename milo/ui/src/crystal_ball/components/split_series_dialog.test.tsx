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
  PerfChartSeries,
  ListMeasurementFilterColumnsResponse,
  SuggestMeasurementFilterValuesResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { SplitSeriesDialog } from './split_series_dialog';

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
            applicableScopes: [],
          },
          {
            column: 'atp_test_name',
            dataType: 1,
            sampleValues: [],
            primary: true,
            isMetricKey: false,
            applicableScopes: [],
          },
        ],
        nextPageToken: '',
      },
      isLoading: false,
    } as unknown as UseQueryResult<ListMeasurementFilterColumnsResponse>);

    mockedSuggestValues.mockReturnValue({
      data: {
        values: ['value1', 'value2'],
        suggestions: [],
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

    expect(screen.getByText('Split Series')).toBeInTheDocument();
    expect(
      screen.getByLabelText('Select Dimension to Split On'),
    ).toBeInTheDocument();
    expect(screen.getByText('Cancel')).toBeInTheDocument();
    expect(screen.getByText('Split')).toBeInTheDocument();
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

    expect(screen.queryByText('Split Series')).not.toBeInTheDocument();
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
        screen.getByRole('option', { name: 'build_branch' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'build_branch' }));

    // Wait for values input to appear
    await waitFor(() => {
      expect(screen.getByLabelText('Select Values')).toBeInTheDocument();
    });

    expect(mockedSuggestValues).toHaveBeenCalledWith(
      expect.objectContaining({ column: 'build_branch' }),
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
        screen.getByRole('option', { name: 'build_branch' }),
      ).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole('option', { name: 'build_branch' }));

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
    fireEvent.click(screen.getByText('Split'));

    expect(mockOnSplit).toHaveBeenCalledWith(['value1'], 'build_branch');
    expect(mockOnClose).toHaveBeenCalledTimes(1);
  });
});
