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
import { render, screen } from '@testing-library/react';

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

describe('<DemoPage />', () => {
  it('should render the demo page', () => {
    mockUseSearchMeasurements.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      isSuccess: false,
      isFetching: false,
    } as UseQueryResult<SearchMeasurementsResponse, Error>);

    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/ui/labs/crystal-ball/demo'],
        }}
        mountedPath="/ui/labs/crystal-ball/demo"
      >
        <TopBarProvider>
          <TopBar />
          <DemoPage />
        </TopBarProvider>
      </FakeContextProvider>,
    );

    expect(screen.getByLabelText('Test Name Filter')).toBeInTheDocument();
    expect(
      screen.getByText('Crystal Ball Performance Metrics'),
    ).toBeInTheDocument();
  });
});
