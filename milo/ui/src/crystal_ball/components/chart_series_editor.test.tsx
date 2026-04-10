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

import '@testing-library/jest-dom';

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { useState } from 'react';

import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';
import { Column } from '@/crystal_ball/constants';
import { UseEditorUiStateOptions } from '@/crystal_ball/hooks';
import {
  PerfChartSeries,
  PerfFilter,
  PerfFilterDefault_FilterOperator,
  SuggestMeasurementFilterValuesRequest,
  SuggestMeasurementFilterValuesResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { ChartSeriesEditor, ChartSeriesItem } from './chart_series_editor';

const mockedSuggestValues = jest.fn(
  (
    _request?: SuggestMeasurementFilterValuesRequest,
    _options?: WrapperQueryOptions<SuggestMeasurementFilterValuesResponse>,
  ) => ({
    data: { values: ['suggest1', 'suggest2'] },
    isLoading: false,
    isError: false,
  }),
);

jest.mock('@/crystal_ball/hooks/use_measurement_filter_api', () => ({
  useSuggestMeasurementFilterValues: (
    request?: SuggestMeasurementFilterValuesRequest,
    options?: WrapperQueryOptions<SuggestMeasurementFilterValuesResponse>,
  ) => mockedSuggestValues(request, options),
}));

jest.mock('react-router', () => ({
  ...jest.requireActual('react-router'),
  useParams: () => ({ dashboardId: 'test-dash' }),
}));

jest.mock('@/crystal_ball/hooks', () => ({
  ...jest.requireActual('@/crystal_ball/hooks'),
  useEditorUiState: ({ initialValue = false }: UseEditorUiStateOptions) => {
    const [val, setVal] = useState(initialValue);
    return [val, setVal];
  },
}));

let mockUUIDCount = 0;
const expectedMockUUID = 'a-b-c-d-1';
beforeAll(() => {
  jest.spyOn(self.crypto, 'randomUUID').mockImplementation(() => {
    mockUUIDCount++;
    return `a-b-c-d-${mockUUIDCount}`;
  });
});

afterAll(() => {
  jest.restoreAllMocks();
});

const defaultProps = {
  series: [] as PerfChartSeries[],
  onUpdateSeries: jest.fn(),
  dataSpecId: 'test-spec-id',
  filterColumns: [],
};

describe('ChartSeriesEditor', () => {
  beforeEach(() => {
    defaultProps.onUpdateSeries.mockClear();
    mockUUIDCount = 0;
    (self.crypto.randomUUID as jest.Mock).mockClear();
    mockedSuggestValues.mockClear();
  });

  it('renders with no series initially', async () => {
    render(<ChartSeriesEditor {...defaultProps} />);
    expect(
      screen.getByRole('button', { name: /Add\/Filter Metric Series/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: /Remove series/i }),
    ).not.toBeInTheDocument();
  });

  it('renders with initial series', async () => {
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 'Series 1',
        metricField: 'metric1',
        dataSpecId: 'test-spec-id',
      }),
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);

    // Check display name when collapsed
    expect(screen.getByText('Series 1')).toBeInTheDocument();

    // Expand the accordion
    fireEvent.click(screen.getByText('Series 1'));
    await waitFor(() => {
      expect(screen.getByLabelText('Metric Field')).toBeInTheDocument();
    });

    expect(screen.getByLabelText('Metric Field')).toHaveValue('metric1');
  });

  it('adds a new series when "Add/Filter Metric Series" is clicked', async () => {
    render(<ChartSeriesEditor {...defaultProps} />);
    fireEvent.click(
      screen.getByRole('button', { name: /Add\/Filter Metric Series/i }),
    );

    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const updatedSeries = defaultProps.onUpdateSeries.mock.lastCall[0];
    expect(updatedSeries.length).toBe(1);
    expect(updatedSeries[0]).toMatchObject({
      displayName: `series-${expectedMockUUID}`,
      metricField: '',
      dataSpecId: 'test-spec-id',
    });
  });

  it('removes a series when delete icon is clicked', async () => {
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 'Series 1',
        metricField: 'metric1',
        dataSpecId: 'test-spec-id',
      }),
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);

    // Expand the accordion to see details (with delete button)
    fireEvent.click(screen.getByText('Series 1'));
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Remove series/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Remove series/i }));

    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledWith([]);
  });

  it('updates series metricField only onBlur', async () => {
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 'Series 1',
        metricField: 'initialMetric',
        dataSpecId: 'test-spec-id',
      }),
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);

    // Expand the accordion
    fireEvent.click(screen.getByText('Series 1'));
    const metricInput = await screen.findByLabelText('Metric Field');

    fireEvent.change(metricInput, { target: { value: 'newMetric' } });
    // onUpdateSeries should not be called yet
    expect(defaultProps.onUpdateSeries).not.toHaveBeenCalled();
    expect(metricInput).toHaveValue('newMetric');

    fireEvent.blur(metricInput);
    // Now onUpdateSeries should be called
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const updatedSeries = defaultProps.onUpdateSeries.mock.lastCall[0];
    expect(updatedSeries[0].metricField).toBe('newMetric');
    // Check if displayName is also updated
    expect(updatedSeries[0].displayName).toBe('Series 1');
  });

  it('does not call onUpdateSeries onBlur if metricField is unchanged', async () => {
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 'Series 1',
        metricField: 'initialMetric',
        dataSpecId: 'test-spec-id',
      }),
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);

    // Expand the accordion
    fireEvent.click(screen.getByText('Series 1'));
    const metricInput = await screen.findByLabelText('Metric Field');

    fireEvent.focus(metricInput);
    fireEvent.blur(metricInput);
    // onUpdateSeries should not be called as the value didn't change
    expect(defaultProps.onUpdateSeries).not.toHaveBeenCalled();
  });

  it('includes global filters and widget filters for atp_test_name in suggestions', async () => {
    const globalFilters: PerfFilter[] = [
      {
        id: 'global-filter-1',
        column: Column.ATP_TEST_NAME,
        dataSpecId: 'test-spec-id',
        displayName: 'Global Atp Test Name',
        textInput: {
          defaultValue: {
            values: ['globalValue'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const widgetFilters: PerfFilter[] = [
      {
        id: 'widget-filter-1',
        column: Column.ATP_TEST_NAME,
        dataSpecId: 'test-spec-id',
        displayName: 'Widget Atp Test Name',
        textInput: {
          defaultValue: {
            values: ['widgetValue'],
            filterOperator: PerfFilterDefault_FilterOperator.EQUAL,
          },
        },
      },
    ];
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 'Series 1',
        metricField: 'metric1',
        dataSpecId: 'test-spec-id',
      }),
    ];
    render(
      <ChartSeriesEditor
        {...defaultProps}
        series={initialSeries}
        globalFilters={globalFilters}
        widgetFilters={widgetFilters}
      />,
    );

    // Expand the accordion
    fireEvent.click(screen.getByText('Series 1'));
    const metricInput = await screen.findByLabelText('Metric Field');

    fireEvent.focus(metricInput);
    fireEvent.change(metricInput, { target: { value: 'newMetric' } });

    await waitFor(() => {
      expect(mockedSuggestValues).toHaveBeenCalledWith(
        expect.objectContaining({
          filter:
            'atp_test_name = "globalValue" AND atp_test_name = "widgetValue"',
        }),
        expect.objectContaining({ enabled: true }),
      );
    });
  });

  it('handles multiple series correctly', async () => {
    const initialSeries: PerfChartSeries[] = [
      PerfChartSeries.fromPartial({
        displayName: 's1',
        metricField: 'm1',
        dataSpecId: 'test-spec-id',
      }),
      PerfChartSeries.fromPartial({
        displayName: 's2',
        metricField: 'm2',
        dataSpecId: 'test-spec-id',
      }),
    ];
    const { rerender } = render(
      <ChartSeriesEditor {...defaultProps} series={initialSeries} />,
    );

    // Expand first series to make its remove button accessible
    fireEvent.click(screen.getByText('s1'));

    // Remove the first one (now that it's expanded and accessible)
    const removeButtons = await screen.findAllByRole('button', {
      name: /Remove series/i,
    });
    fireEvent.click(removeButtons[0]);
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const seriesAfterRemove = defaultProps.onUpdateSeries.mock.lastCall[0];
    expect(seriesAfterRemove.length).toBe(1);
    expect(seriesAfterRemove[0].metricField).toBe('m2');

    // Simulate parent re-rendering with the new series prop
    rerender(
      <ChartSeriesEditor {...defaultProps} series={seriesAfterRemove} />,
    );

    // Add a new one
    fireEvent.click(
      screen.getByRole('button', { name: /Add\/Filter Metric Series/i }),
    );
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(2);
    const seriesAfterAdd = defaultProps.onUpdateSeries.mock.lastCall[0];
    expect(seriesAfterAdd.length).toBe(2);
    expect(seriesAfterAdd[0].metricField).toBe('m2'); // Existing
    expect(seriesAfterAdd[1].metricField).toBe(''); // New
    expect(seriesAfterAdd[1].displayName).toBe(`series-${expectedMockUUID}`);
  });
});

describe('ChartSeriesItem', () => {
  const itemProps = {
    series: PerfChartSeries.fromPartial({
      displayName: 'Item 1',
      metricField: 'metric1',
      dataSpecId: 'test-spec-id',
    }),
    onUpdate: jest.fn(),
    onRemove: jest.fn(),
    dataSpecId: 'test-spec-id',
    metricFilterColumns: [],
    uiStateOptions: { key: 'test-persist-key' },
  };

  beforeEach(() => {
    itemProps.onUpdate.mockClear();
    itemProps.onRemove.mockClear();
  });

  it('hides color options when hideColorPicker is true', async () => {
    render(<ChartSeriesItem {...itemProps} hideColorPicker={true} />);
    expect(screen.queryByTestId('series-color-circle')).not.toBeInTheDocument();

    fireEvent.click(screen.getByText('Item 1'));
    await waitFor(() => {
      expect(screen.getByLabelText('Metric Field')).toBeInTheDocument();
    });
    expect(screen.queryByLabelText('Color')).not.toBeInTheDocument();
  });

  it('shows color options when hideColorPicker is false (default)', async () => {
    render(<ChartSeriesItem {...itemProps} />);
    expect(screen.getByTestId('series-color-circle')).toBeInTheDocument();

    fireEvent.click(screen.getByText('Item 1'));
    await waitFor(() => {
      expect(screen.getByLabelText('Metric Field')).toBeInTheDocument();
    });
    expect(screen.getByLabelText('Color')).toBeInTheDocument();
  });
});
