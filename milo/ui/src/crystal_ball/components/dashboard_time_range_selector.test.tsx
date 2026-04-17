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

import { fireEvent, render, screen } from '@testing-library/react';
import { DateTime } from 'luxon';

import { COMMON_MESSAGES } from '@/crystal_ball/constants';
import { DashboardState } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { DashboardTimeRangeSelector } from './dashboard_time_range_selector';

interface MockDateTimePickerProps {
  label: string;
  onChange: (value: DateTime | null) => void;
  slotProps?: {
    textField?: {
      helperText?: string | null;
      error?: boolean;
    };
  };
}

// Mock DateTimePicker to avoid issues with complex MUI component in tests
jest.mock('@mui/x-date-pickers/DateTimePicker', () => {
  return {
    DateTimePicker: (props: MockDateTimePickerProps) => {
      return (
        <div>
          <label htmlFor={props.label}>{props.label}</label>
          <input
            id={props.label}
            type="text"
            onChange={(e) => {
              const val = e.target.value;
              props.onChange(val ? DateTime.fromISO(val) : null);
            }}
          />
          {props.slotProps?.textField?.helperText && (
            <span>{props.slotProps.textField.helperText}</span>
          )}
        </div>
      );
    },
  };
});

describe('DashboardTimeRangeSelector', () => {
  const mockOnApply = jest.fn();
  const defaultState = DashboardState.fromPartial({
    dashboardContent: {
      globalFilters: [],
      widgets: [],
      dataSpecs: {},
    },
  });

  const renderComponent = (state = defaultState) => {
    return render(
      <DashboardTimeRangeSelector
        dashboardState={state}
        onApply={mockOnApply}
      />,
    );
  };

  beforeEach(() => {
    mockOnApply.mockClear();
  });

  it('renders the time button', () => {
    renderComponent();
    expect(screen.getByTestId('time-button')).toBeInTheDocument();
  });

  it('opens popover on click', () => {
    renderComponent();
    fireEvent.click(screen.getByTestId('time-button'));
    expect(screen.getByText('Relative Range')).toBeInTheDocument();
    expect(screen.getByText('Custom Range (UTC)')).toBeInTheDocument();
  });

  it('shows errors when custom range is applied with missing dates', () => {
    renderComponent();
    fireEvent.click(screen.getByTestId('time-button'));

    // Click Apply without filling dates
    const applyButtons = screen.getAllByText('Apply');
    // The second one should be for custom range
    fireEvent.click(applyButtons[1]);

    expect(
      screen.getByText(COMMON_MESSAGES.FROM_DATE_REQUIRED),
    ).toBeInTheDocument();
    expect(
      screen.getByText(COMMON_MESSAGES.TO_DATE_REQUIRED),
    ).toBeInTheDocument();
    expect(mockOnApply).not.toHaveBeenCalled();
  });

  it('shows errors when start date is after end date', () => {
    renderComponent();
    fireEvent.click(screen.getByTestId('time-button'));

    const fromInput = screen.getByLabelText('From (UTC)');
    const toInput = screen.getByLabelText('To (UTC)');

    // Use valid ISO strings that Luxon can parse
    fireEvent.change(fromInput, { target: { value: '2026-04-16T12:00:00Z' } });
    fireEvent.change(toInput, { target: { value: '2026-04-15T12:00:00Z' } });

    const applyButtons = screen.getAllByText('Apply');
    fireEvent.click(applyButtons[1]);

    expect(
      screen.getByText(COMMON_MESSAGES.FROM_DATE_MUST_BE_BEFORE_TO),
    ).toBeInTheDocument();
    expect(
      screen.getByText(COMMON_MESSAGES.TO_DATE_MUST_BE_AFTER_FROM),
    ).toBeInTheDocument();
    expect(mockOnApply).not.toHaveBeenCalled();
  });

  it('calls onApply when valid custom range is specified', () => {
    renderComponent();
    fireEvent.click(screen.getByTestId('time-button'));

    const fromInput = screen.getByLabelText('From (UTC)');
    const toInput = screen.getByLabelText('To (UTC)');

    fireEvent.change(fromInput, { target: { value: '2026-04-15T12:00:00Z' } });
    fireEvent.change(toInput, { target: { value: '2026-04-16T12:00:00Z' } });

    const applyButtons = screen.getAllByText('Apply');
    fireEvent.click(applyButtons[1]);

    expect(mockOnApply).toHaveBeenCalled();
  });
});
