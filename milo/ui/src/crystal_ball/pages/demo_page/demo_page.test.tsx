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
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { DateTime } from 'luxon';

import { TopBar } from '@/crystal_ball/components/layout/top_bar';
import { TopBarProvider } from '@/crystal_ball/components/layout/top_bar_provider';
import * as useAndroidPerfApi from '@/crystal_ball/hooks/use_android_perf_api';
import { SearchMeasurementsResponse } from '@/crystal_ball/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { DemoPage } from './demo_page';

// Mock the hook
const mockUseSearchMeasurements = jest.spyOn(
  useAndroidPerfApi,
  'useSearchMeasurements',
);

describe('<DemoPage />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const renderDemoPage = (
    initialUrl: string = '/ui/labs/crystal-ball/demo',
  ) => {
    return render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: [initialUrl],
        }}
        mountedPath="/ui/labs/crystal-ball/demo"
      >
        <TopBarProvider>
          <TopBar />
          <DemoPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );
  };

  const defaultMockReturn = {
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
    isSuccess: false,
    isFetching: false,
  } as UseQueryResult<SearchMeasurementsResponse, Error>;

  it('renders the initial empty state when no search params are provided', () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    renderDemoPage();

    expect(
      screen.getByText('Crystal Ball Performance Metrics'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Enter search parameters to view performance data.'),
    ).toBeInTheDocument();
  });

  it('displays a loading spinner while fetching data', () => {
    mockUseSearchMeasurements.mockReturnValue({
      ...defaultMockReturn,
      isLoading: true,
    } as unknown as UseQueryResult<SearchMeasurementsResponse, Error>);

    // Provide a URL so searchRequest gets populated and triggers the loading state branch
    renderDemoPage('/ui/labs/crystal-ball/demo?q=metricKeys%3A%22cpu%22');

    expect(screen.getByRole('progressbar')).toBeInTheDocument();
    expect(
      screen.queryByText('Enter search parameters to view performance data.'),
    ).not.toBeInTheDocument();
  });

  it('displays an error alert when the API request fails', async () => {
    mockUseSearchMeasurements.mockReturnValue({
      ...defaultMockReturn,
      isError: true,
      error: new Error('Backend timeout'),
    } as unknown as UseQueryResult<SearchMeasurementsResponse, Error>);

    renderDemoPage('/ui/labs/crystal-ball/demo?q=metricKeys%3A%22cpu%22');

    expect(screen.getByRole('alert')).toHaveTextContent(
      'Error fetching measurements: Backend timeout',
    );
  });

  it('displays a no data message when the search returns empty rows', async () => {
    mockUseSearchMeasurements.mockReturnValue({
      ...defaultMockReturn,
      data: { rows: [], nextPageToken: '' },
    } as unknown as UseQueryResult<SearchMeasurementsResponse, Error>);

    renderDemoPage('/ui/labs/crystal-ball/demo?q=metricKeys%3A%22cpu%22');

    expect(
      await screen.findByText('No data found for the given parameters.'),
    ).toBeInTheDocument();
  });

  it('renders the chart when data is successfully fetched', async () => {
    mockUseSearchMeasurements.mockReturnValue({
      ...defaultMockReturn,
      data: {
        nextPageToken: '',
        rows: [
          {
            buildCreateTime: '2026-02-24T10:00:00Z',
            metricKey: 'cpu',
            value: 45.5,
            buildId: '12345',
          },
        ],
      },
    } as unknown as UseQueryResult<SearchMeasurementsResponse, Error>);

    renderDemoPage('/ui/labs/crystal-ball/demo?q=metricKeys%3A%22cpu%22');

    // Assumes TimeSeriesChart renders this test-id when populated
    const element = await screen.findByTestId('time-series-chart');
    expect(element).toBeInTheDocument();
  });

  it('syncs TimeRangeSelector URL params into the API request', async () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    const now = DateTime.now();
    const startTimeUnix = now.minus({ days: 3 }).toUTC().toUnixInteger();
    const endTimeUnix = now.toUTC().toUnixInteger();
    const initialUrl =
      `/ui/labs/crystal-ball/demo?time_option=customize` +
      `&start_time=${startTimeUnix}&end_time=${endTimeUnix}&q=metricKeys%3A%22cpu%22`;

    renderDemoPage(initialUrl);

    await waitFor(() => {
      expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
        expect.objectContaining({
          metricKeys: ['cpu'],
          buildCreateStartTime: { seconds: startTimeUnix, nanos: 0 },
          buildCreateEndTime: { seconds: endTimeUnix, nanos: 0 },
        }),
        expect.anything(),
      );
    });
  });

  it('does not crash and passes default time range to hook on initial load', async () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    renderDemoPage();

    expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
      expect.objectContaining({
        buildCreateStartTime: expect.any(Object),
        buildCreateEndTime: expect.any(Object),
      }),
      expect.objectContaining({ enabled: false }),
    );

    expect(
      screen.getByText('Enter search parameters to view performance data.'),
    ).toBeInTheDocument();
  });

  it('correctly initializes search request from URL with all parameters filled out', async () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    const now = DateTime.now();
    const startTimeUnix = now.minus({ days: 5 }).toUTC().toUnixInteger();
    const endTimeUnix = now.minus({ days: 1 }).toUTC().toUnixInteger();

    const query =
      'testNameFilter:MyTest buildBranch:main buildTarget:x86 metricKeys:cpu,memory atpTestNameFilter:AtpTest';
    const encodedQuery = encodeURIComponent(query);

    const initialUrl =
      `/ui/labs/crystal-ball/demo?time_option=customize` +
      `&start_time=${startTimeUnix}&end_time=${endTimeUnix}&q=${encodedQuery}`;

    renderDemoPage(initialUrl);

    await waitFor(() => {
      expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
        expect.objectContaining({
          testNameFilter: 'MyTest',
          buildBranch: 'main',
          buildTarget: 'x86',
          metricKeys: ['cpu', 'memory'],
          atpTestNameFilter: 'AtpTest',
          buildCreateStartTime: { seconds: startTimeUnix, nanos: 0 },
          buildCreateEndTime: { seconds: endTimeUnix, nanos: 0 },
        }),
        expect.anything(),
      );
    });

    expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({ enabled: true }),
    );
  });

  it('correctly applies default time range when missing from URL but other query parameters are present', async () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    const query = 'metricKeys:cpu';
    const encodedQuery = encodeURIComponent(query);
    const initialUrl = `/ui/labs/crystal-ball/demo?q=${encodedQuery}`;

    renderDemoPage(initialUrl);

    const now = DateTime.now();
    const expectedStartTimeUnix = now
      .minus({ days: 3 })
      .toUTC()
      .toUnixInteger();
    const expectedEndTimeUnix = now.toUTC().toUnixInteger();

    await waitFor(() => {
      expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
        expect.objectContaining({
          metricKeys: ['cpu'],
          buildCreateStartTime: expect.objectContaining({
            seconds: expect.closeTo(expectedStartTimeUnix, 2),
            nanos: 0,
          }),
          buildCreateEndTime: expect.objectContaining({
            seconds: expect.closeTo(expectedEndTimeUnix, 2),
            nanos: 0,
          }),
        }),
        expect.anything(),
      );
    });
  });

  it('updates the API request when submitting the form while preserving top bar dates', async () => {
    mockUseSearchMeasurements.mockReturnValue(defaultMockReturn);

    const now = DateTime.now();
    const startTimeUnix = now.minus({ days: 1 }).toUTC().toUnixInteger();
    const endTimeUnix = now.toUTC().toUnixInteger();

    renderDemoPage(
      `/ui/labs/crystal-ball/demo?time_option=customize&start_time=${startTimeUnix}&end_time=${endTimeUnix}`,
    );

    fireEvent.change(screen.getByLabelText('Test Name Filter'), {
      target: { value: 'MyNewTest' },
    });

    const metricInput = screen.getByLabelText('Add Metric Key *');
    fireEvent.change(metricInput, { target: { value: 'memory' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(mockUseSearchMeasurements).toHaveBeenCalledWith(
        expect.objectContaining({
          testNameFilter: 'MyNewTest',
          metricKeys: ['memory'],
          buildCreateStartTime: { seconds: startTimeUnix, nanos: 0 },
          buildCreateEndTime: { seconds: endTimeUnix, nanos: 0 },
        }),
        expect.anything(),
      );
    });
  });
});
