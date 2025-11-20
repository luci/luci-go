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

import { AdapterLuxon } from '@mui/x-date-pickers/AdapterLuxon';
import { LocalizationProvider } from '@mui/x-date-pickers/LocalizationProvider';
import { UseQueryResult } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';

import * as useAndroidPerfApi from '@/crystal_ball/hooks/use_android_perf_api';

import { LandingPage } from './landing_page';

// Mock the hook
const mockUseSearchMeasurements = jest.spyOn(
  useAndroidPerfApi,
  'useSearchMeasurements',
);

// Mock DateTimePicker as it's used in the child form
jest.mock('@mui/x-date-pickers/DateTimePicker', () => ({
  DateTimePicker: jest.fn(({ label, value, onChange, disabled }) => (
    <input
      type="text"
      aria-label={label}
      value={value ? value.toISO() : ''}
      onChange={(e) => onChange(e.target.value)}
      disabled={disabled}
    />
  )),
}));

const renderWithProvider = (ui: React.ReactElement) => {
  return render(
    <LocalizationProvider dateAdapter={AdapterLuxon}>
      {ui}
    </LocalizationProvider>,
  );
};

describe('<LandingPage />', () => {
  it('should render the landing page', () => {
    mockUseSearchMeasurements.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      isSuccess: false,
      isFetching: false,
    } as UseQueryResult<useAndroidPerfApi.SearchMeasurementsResponse, Error>);

    renderWithProvider(<LandingPage />);
    expect(
      screen.getByText(/Crystal Ball Performance Metrics/i),
    ).toBeInTheDocument();
    // Check that the form is rendered by looking for one of its labels
    expect(screen.getByLabelText('Test Name Filter')).toBeInTheDocument();
  });
});
