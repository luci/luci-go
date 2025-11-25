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

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { DateTime } from 'luxon';

import { SearchMeasurementsForm } from '@/crystal_ball/components';
import { MAXIMUM_PAGE_SIZE } from '@/crystal_ball/constants';
import { SearchMeasurementsRequest } from '@/crystal_ball/types';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

// Mock DateTimePicker to just render a text field
jest.mock('@mui/x-date-pickers/DateTimePicker', () => ({
  DateTimePicker: jest.fn(({ label, value, onChange, disabled, slotProps }) => (
    <input
      type="text"
      aria-label={label}
      value={value ? value.toISO() : ''}
      onChange={(e) => {
        const newValue = DateTime.fromISO(e.target.value);
        if (newValue.isValid) {
          onChange(newValue);
        } else {
          onChange(null);
        }
      }}
      disabled={disabled}
      data-testid={`${label}-input`}
      style={slotProps?.textField?.error ? { border: '1px solid red' } : {}}
    />
  )),
}));

const renderWithProvider = (ui: React.ReactElement) => {
  return render(<FakeContextProvider>{ui}</FakeContextProvider>);
};

// Stable empty object for initialRequest
const emptyInitialRequest = {};

describe('SearchMeasurementsForm', () => {
  const mockOnSubmit = jest.fn();

  beforeEach(() => {
    mockOnSubmit.mockClear();
  });

  it('renders all form fields', () => {
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
    expect(screen.getByLabelText('Last N Days')).toBeInTheDocument();
    expect(
      screen.getByLabelText('Build Create Start Time'),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Build Create End Time')).toBeInTheDocument();
    expect(screen.getByLabelText('Add Metric Key *')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Search' })).toBeInTheDocument();
  });

  it('updates text fields on user input', () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    const testNameInput = screen.getByLabelText('Test Name Filter');
    fireEvent.change(testNameInput, { target: { value: 'New Test' } });
    expect(testNameInput).toHaveValue('New Test');

    const branchInput = screen.getByLabelText('Build Branch');
    fireEvent.change(branchInput, { target: { value: 'main' } });
    expect(branchInput).toHaveValue('main');
  });

  it('adds and removes metric key chips', async () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );
    const metricInput = screen.getByLabelText('Add Metric Key *');

    fireEvent.change(metricInput, { target: { value: 'metric1' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

    expect(metricInput).toHaveValue('');
    const chip1 = await screen.findByText('metric1');
    expect(chip1).toBeInTheDocument();

    // Remove metric1
    const deleteIcon = screen.getByTestId('CancelIcon');
    if (deleteIcon) {
      fireEvent.click(deleteIcon);
    }
    await waitFor(() => {
      expect(screen.queryByText('metric1')).not.toBeInTheDocument();
    });

    fireEvent.change(metricInput, { target: { value: 'metric2' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });
    const chip2 = await screen.findByText('metric2');
    expect(chip2).toBeInTheDocument();

    expect(screen.getByText('metric2')).toBeInTheDocument();
  });

  it('disables date pickers when Last N Days is filled', () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    const lastNDaysInput = screen.getByLabelText('Last N Days');
    const startTimeInput = screen.getByLabelText('Build Create Start Time');
    const endTimeInput = screen.getByLabelText('Build Create End Time');

    expect(startTimeInput).not.toBeDisabled();
    expect(endTimeInput).not.toBeDisabled();

    fireEvent.change(lastNDaysInput, { target: { value: '7' } });

    expect(startTimeInput).toBeDisabled();
    expect(endTimeInput).toBeDisabled();

    fireEvent.change(lastNDaysInput, { target: { value: '' } });

    expect(startTimeInput).not.toBeDisabled();
    expect(endTimeInput).not.toBeDisabled();
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
    fireEvent.change(screen.getByLabelText('Last N Days'), {
      target: { value: '14' },
    });

    const metricInput = screen.getByLabelText('Add Metric Key *');
    fireEvent.change(metricInput, { target: { value: 'cpu_usage' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledTimes(1);
      expect(mockOnSubmit).toHaveBeenCalledWith({
        testNameFilter: 'MyTest',
        buildCreateStartTime: undefined,
        buildCreateEndTime: undefined,
        lastNDays: 14,
        buildBranch: 'release',
        buildTarget: undefined,
        atpTestNameFilter: undefined,
        metricKeys: ['cpu_usage'],
        extraColumns: undefined,
        pageSize: MAXIMUM_PAGE_SIZE,
      });
    });
  });

  it('calls onSubmit with date range when Last N Days is not set', async () => {
    renderWithProvider(
      <SearchMeasurementsForm
        onSubmit={mockOnSubmit}
        initialRequest={emptyInitialRequest}
      />,
    );

    const startTimeInput = screen.getByTestId('Build Create Start Time-input');
    const endTimeInput = screen.getByTestId('Build Create End Time-input');
    const metricInput = screen.getByLabelText('Add Metric Key *');

    const startTime = DateTime.fromISO('2025-11-10T00:00:00Z');
    const endTime = DateTime.fromISO('2025-11-15T00:00:00Z');

    fireEvent.change(startTimeInput, { target: { value: startTime.toISO() } });
    fireEvent.change(endTimeInput, { target: { value: endTime.toISO() } });
    fireEvent.change(metricInput, { target: { value: 'memory' } });
    fireEvent.keyDown(metricInput, { key: 'Enter', code: 'Enter' });

    fireEvent.click(screen.getByRole('button', { name: 'Search' }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalledTimes(1);
      expect(mockOnSubmit).toHaveBeenCalledWith(
        expect.objectContaining({
          buildCreateStartTime: { seconds: startTime.toSeconds(), nanos: 0 },
          buildCreateEndTime: { seconds: endTime.toSeconds(), nanos: 0 },
          lastNDays: undefined,
          metricKeys: ['memory'],
        }),
      );
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
      lastNDays: 5,
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
    expect(screen.getByLabelText('Last N Days')).toHaveValue(5);
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
      expect(
        screen.getByText(
          'Please specify either "Last N Days" or both a Start and End Time.',
        ),
      ).toBeInTheDocument();
      expect(mockOnSubmit).not.toHaveBeenCalled();
    });

    it('shows error if only start time is provided', async () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={{ metricKeys: ['test'] }}
        />,
      );

      const startTimeInput = screen.getByTestId(
        'Build Create Start Time-input',
      );
      fireEvent.change(startTimeInput, {
        target: { value: DateTime.fromISO('2025-11-10T00:00:00Z').toISO() },
      });

      fireEvent.click(screen.getByRole('button', { name: 'Search' }));

      expect(
        await screen.findByText(
          'Please specify either "Last N Days" or both a Start and End Time.',
        ),
      ).toBeInTheDocument();
      expect(mockOnSubmit).not.toHaveBeenCalled();
    });

    it('shows error if start time is after end time', async () => {
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={{ metricKeys: ['test'] }}
        />,
      );

      const startTimeInput = screen.getByTestId(
        'Build Create Start Time-input',
      );
      const endTimeInput = screen.getByTestId('Build Create End Time-input');

      fireEvent.change(startTimeInput, {
        target: { value: DateTime.fromISO('2025-11-15T00:00:00Z').toISO() },
      });
      fireEvent.change(endTimeInput, {
        target: { value: DateTime.fromISO('2025-11-10T00:00:00Z').toISO() },
      });

      fireEvent.click(screen.getByRole('button', { name: 'Search' }));

      expect(
        await screen.findByText('Start Time must be before End Time.'),
      ).toBeInTheDocument();
      expect(mockOnSubmit).not.toHaveBeenCalled();
    });

    it('shows errors on load if initialRequest is invalid', () => {
      const invalidInitialRequest: Partial<SearchMeasurementsRequest> = {
        lastNDays: 0, // Invalid value
      };
      renderWithProvider(
        <SearchMeasurementsForm
          onSubmit={mockOnSubmit}
          initialRequest={invalidInitialRequest}
        />,
      );

      expect(
        screen.getByText('At least one metric key is required.'),
      ).toBeInTheDocument();
      expect(
        screen.getByText(
          'Please specify either "Last N Days" or both a Start and End Time.',
        ),
      ).toBeInTheDocument();
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

      fireEvent.change(screen.getByLabelText('Last N Days'), {
        target: { value: '3' },
      });
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
