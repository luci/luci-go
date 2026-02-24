// Copyright 2025 The LUCI Authors.
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

import { LocalizationProvider } from '@mui/x-date-pickers';
import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { SearchMeasurementsForm } from '@/crystal_ball/components';
import { MAXIMUM_PAGE_SIZE } from '@/crystal_ball/constants';
import { SearchMeasurementsRequest } from '@/crystal_ball/types';

describe('SearchMeasurementsForm', () => {
  const mockOnSubmit = jest.fn();
  const emptyInitialRequest: Partial<SearchMeasurementsRequest> = {};

  const renderWithProvider = (ui: React.ReactElement) => {
    return render(
      <LocalizationProvider dateAdapter={AdapterLuxon}>
        {ui}
      </LocalizationProvider>,
    );
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders standard form elements', () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    expect(screen.getByLabelText('Test Name Filter')).toBeInTheDocument();
    expect(screen.getByLabelText('ATP Test Name Filter')).toBeInTheDocument();
    expect(screen.getByLabelText('Build Branch')).toBeInTheDocument();
    expect(screen.getByLabelText('Build Target')).toBeInTheDocument();
    expect(screen.getByLabelText('Add Metric Key *')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument();
  });

  it('adds and removes metric keys', () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    const metricInput = screen.getByLabelText('Add Metric Key *');

    fireEvent.change(metricInput, { target: { value: 'metric_a' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });
    expect(screen.getByText('metric_a')).toBeInTheDocument();

    fireEvent.change(metricInput, { target: { value: 'metric_b' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });
    expect(screen.getByText('metric_b')).toBeInTheDocument();

    // Remove metric_a
    const deleteButtonForA = screen
      .getByText('metric_a')
      .parentElement!.querySelector('.MuiChip-deleteIcon');
    fireEvent.click(deleteButtonForA!);

    expect(screen.queryByText('metric_a')).not.toBeInTheDocument();
    expect(screen.getByText('metric_b')).toBeInTheDocument();
  });

  it('calls onSubmit with correct values', async () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    fireEvent.change(screen.getByLabelText('Test Name Filter'), {
      target: { value: 'MyTest' },
    });
    fireEvent.change(screen.getByLabelText('Build Branch'), {
      target: { value: 'release' },
    });

    const metricInput = screen.getByLabelText('Add Metric Key *');
    fireEvent.change(metricInput, { target: { value: 'cpu_usage' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledTimes(1);
      expect(mockOnSubmit).toHaveBeenCalledWith({
        testNameFilter: 'MyTest',
        buildBranch: 'release',
        buildTarget: undefined,
        atpTestNameFilter: undefined,
        metricKeys: ['cpu_usage'],
        pageSize: MAXIMUM_PAGE_SIZE,
      });
    });
  });

  it('disables submit button when isSubmitting is true', () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        isSubmitting={true}
        initialRequest={emptyInitialRequest}
      />,
    );
    expect(screen.getByRole('button', { name: 'Searching...' })).toBeDisabled();
  });

  it('loads initial values from initialRequest prop', () => {
    const initialRequest: Partial<SearchMeasurementsRequest> = {
      testNameFilter: 'InitialTest',
      metricKeys: ['init_metric'],
    };
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={initialRequest}
      />,
    );

    expect(screen.getByLabelText('Test Name Filter')).toHaveValue(
      'InitialTest',
    );
    expect(screen.getByText('init_metric')).toBeInTheDocument();
  });

  describe('Validation', () => {
    it('does not show errors on initial render with empty request', () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={emptyInitialRequest}
        />,
      );
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });

    it('shows errors on submit if fields are missing', async () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={emptyInitialRequest}
        />,
      );
      fireEvent.click(screen.getByRole('button', { name: 'Search' }));

      expect(
        await screen.findByText('At least one metric key is required.'),
      ).toBeInTheDocument();
      expect(mockOnSubmit).not.toHaveBeenCalled();
    });

    it('does not submit if form is invalid', async () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={emptyInitialRequest}
        />,
      );
      fireEvent.click(screen.getByRole('button', { name: 'Search' }));
      await screen.findByText('At least one metric key is required.');
      expect(mockOnSubmit).not.toHaveBeenCalled();
    });

    it('submits if form is valid', async () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={emptyInitialRequest}
        />,
      );

      const metricInput = screen.getByLabelText('Add Metric Key *');
      fireEvent.change(metricInput, { target: { value: 'test_metric' } });
      fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

      fireEvent.click(screen.getByRole('button', { name: 'Search' }));

      await waitFor(() => {
        expect(mockOnSubmit).toHaveBeenCalledTimes(1);
      });
      expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });
  });
});
