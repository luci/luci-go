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

import { PerfChartSeries } from '@/crystal_ball/types';

import { ChartSeriesEditor } from './chart_series_editor';

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
};

describe('ChartSeriesEditor', () => {
  beforeEach(() => {
    defaultProps.onUpdateSeries.mockClear();
    mockUUIDCount = 0;
    (self.crypto.randomUUID as jest.Mock).mockClear();
  });

  it('renders with no series initially', async () => {
    render(<ChartSeriesEditor {...defaultProps} />);
    expect(screen.getByText('Series')).toBeInTheDocument();

    // Expand the accordion to check inner content
    fireEvent.click(screen.getByText('Series'));
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Add Series/i }),
      ).toBeInTheDocument();
    });
    expect(
      screen.queryByRole('button', { name: /Remove series/i }),
    ).not.toBeInTheDocument();

    // Collapse and check text
    fireEvent.click(screen.getByText('Series'));
    await waitFor(() => {
      expect(screen.getByText('No series added.')).toBeInTheDocument();
    });
  });

  it('renders with initial series', async () => {
    const initialSeries: PerfChartSeries[] = [
      {
        displayName: 'Series 1',
        metricField: 'metric1',
        dataSpecId: 'test-spec-id',
      },
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);

    // Check chip label when collapsed
    expect(screen.getByText('metric1')).toBeInTheDocument();

    // Expand the accordion
    fireEvent.click(screen.getByText('Series'));
    await waitFor(() => {
      expect(screen.getByLabelText('Metric Field')).toBeInTheDocument();
    });

    expect(screen.getByLabelText('Metric Field')).toHaveValue('metric1');
  });

  it('adds a new series when "Add Series" is clicked', async () => {
    render(<ChartSeriesEditor {...defaultProps} />);
    fireEvent.click(screen.getByText('Series')); // Expand
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: /Add Series/i }),
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole('button', { name: /Add Series/i }));

    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const updatedSeries = defaultProps.onUpdateSeries.mock.calls[0][0];
    expect(updatedSeries.length).toBe(1);
    expect(updatedSeries[0]).toMatchObject({
      displayName: `series-${expectedMockUUID}`,
      metricField: '',
      dataSpecId: 'test-spec-id',
    });
  });

  it('removes a series when delete icon is clicked', async () => {
    const initialSeries: PerfChartSeries[] = [
      {
        displayName: 'Series 1',
        metricField: 'metric1',
        dataSpecId: 'test-spec-id',
      },
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);
    fireEvent.click(screen.getByText('Series')); // Expand
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
      {
        displayName: 'Series 1',
        metricField: 'initialMetric',
        dataSpecId: 'test-spec-id',
      },
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);
    fireEvent.click(screen.getByText('Series')); // Expand
    const metricInput = await screen.findByLabelText('Metric Field');

    fireEvent.change(metricInput, { target: { value: 'newMetric' } });
    // onUpdateSeries should not be called yet
    expect(defaultProps.onUpdateSeries).not.toHaveBeenCalled();
    expect(metricInput).toHaveValue('newMetric');

    fireEvent.blur(metricInput);
    // Now onUpdateSeries should be called
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const updatedSeries = defaultProps.onUpdateSeries.mock.calls[0][0];
    expect(updatedSeries[0].metricField).toBe('newMetric');
    // Check if displayName is also updated
    expect(updatedSeries[0].displayName).toBe('Series 1');
  });

  it('does not call onUpdateSeries onBlur if metricField is unchanged', async () => {
    const initialSeries: PerfChartSeries[] = [
      {
        displayName: 'Series 1',
        metricField: 'initialMetric',
        dataSpecId: 'test-spec-id',
      },
    ];
    render(<ChartSeriesEditor {...defaultProps} series={initialSeries} />);
    fireEvent.click(screen.getByText('Series')); // Expand
    const metricInput = await screen.findByLabelText('Metric Field');

    fireEvent.focus(metricInput);
    fireEvent.blur(metricInput);
    // onUpdateSeries should not be called as the value didn't change
    expect(defaultProps.onUpdateSeries).not.toHaveBeenCalled();
  });

  it('handles multiple series correctly', async () => {
    const initialSeries: PerfChartSeries[] = [
      { displayName: 's1', metricField: 'm1', dataSpecId: 'test-spec-id' },
      { displayName: 's2', metricField: 'm2', dataSpecId: 'test-spec-id' },
    ];
    const { rerender } = render(
      <ChartSeriesEditor {...defaultProps} series={initialSeries} />,
    );
    fireEvent.click(screen.getByText('Series')); // Expand
    await waitFor(() => {
      expect(screen.getAllByLabelText('Metric Field').length).toBe(2);
    });

    // Remove the first one
    const removeButtons = screen.getAllByRole('button', {
      name: /Remove series/i,
    });
    fireEvent.click(removeButtons[0]);
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(1);
    const seriesAfterRemove = defaultProps.onUpdateSeries.mock.calls[0][0];
    expect(seriesAfterRemove.length).toBe(1);
    expect(seriesAfterRemove[0].metricField).toBe('m2');

    // Simulate parent re-rendering with the new series prop
    rerender(
      <ChartSeriesEditor {...defaultProps} series={seriesAfterRemove} />,
    );

    // Add a new one
    fireEvent.click(screen.getByRole('button', { name: /Add Series/i }));
    expect(defaultProps.onUpdateSeries).toHaveBeenCalledTimes(2);
    const seriesAfterAdd = defaultProps.onUpdateSeries.mock.calls[1][0];
    expect(seriesAfterAdd.length).toBe(2);
    expect(seriesAfterAdd[0].metricField).toBe('m2'); // Existing
    expect(seriesAfterAdd[1].metricField).toBe(''); // New
    expect(seriesAfterAdd[1].displayName).toBe(`series-${expectedMockUUID}`);
  });
});
