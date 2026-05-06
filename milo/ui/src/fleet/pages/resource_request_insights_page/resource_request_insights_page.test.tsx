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

import { render, screen, waitFor } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { RRI_VERSION_TEST_URLS } from '@/fleet/testing_tools/version_test_urls';
import { GetResourceRequestsMultiselectFilterValuesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Component as RriPageComponent } from './resource_request_insights_page';

const MOCK_FILTER_VALUES =
  GetResourceRequestsMultiselectFilterValuesResponse.fromPartial({
    rrIds: ['RR-001', 'RR-0012'],
    resourceDetails: [
      'FF Creation Testing - [Resource Request Demo][Annual]: ' +
        'Pixel Tablet (2023) x 549 units for xTS Quality Improvement - Devices for automated testing',
      'FF Creation Testing 2 - [Resource Request Demo][Annual]: ' +
        'Pixel Tablet (2023) x 451 units for xTS Quality Improvement - Devices for automated testing',
    ],
    customer: ['ANPLAT', 'BROTHER'],
    resourceGroups: ['ANDROID_TABLET', 'CHROME_DESKTOP'],
  });

jest.mock('@/fleet/hooks/prpc_clients', () => ({
  useFleetConsoleClient: jest.fn(),
}));

describe('<ResourceRequestListPage />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      GetResourceRequestsMultiselectFilterValues: {
        query: () => ({
          queryKey: ['test'],
          queryFn: () => MOCK_FILTER_VALUES,
        }),
      },
      CountResourceRequests: {
        query: () => ({
          queryKey: ['count'],
          queryFn: () => ({ count: 10 }),
        }),
      },
      ListResourceRequests: {
        query: () => ({
          queryKey: ['list'],
          queryFn: () => ({ resourceRequests: [] }),
        }),
      },
    });
  });

  it('renders loading by default', async () => {
    (useFleetConsoleClient as jest.Mock).mockReturnValue({
      GetResourceRequestsMultiselectFilterValues: {
        query: () => ({
          queryKey: ['test'],
          isLoading: true,
          data: undefined,
          queryFn: () => new Promise(() => {}),
        }),
      },
      CountResourceRequests: {
        query: () => ({
          queryKey: ['count'],
          queryFn: () => ({ count: 0 }),
        }),
      },
      ListResourceRequests: {
        query: () => ({
          queryKey: ['list'],
          queryFn: () => ({ resourceRequests: [] }),
        }),
      },
    });

    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <SettingsProvider>
            <RriPageComponent />
          </SettingsProvider>
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(screen.getAllByRole('progressbar').length).toBeGreaterThan(0);
  });

  const testCases = [
    {
      name: 'should redirect from legacy URL to v2 and show chips',
      inputUrl: RRI_VERSION_TEST_URLS.v1,
      expectedChips: [
        '1 | Fulfillment Status IN Not Started',
        '2 | Customer IN ANPLAT, BROTHER',
        '2 | RR ID IN RR-001, RR-0012',
        '2 | Resource Details IN FF Creation Testing - [Resource Request Demo][Annual]: ' +
          'Pixel Tablet (2023) x 549 units for xTS Quality Improvement - Devices for automated testing, ' +
          'FF Creation Testing 2 - [Resource Request Demo][Annual]: ' +
          'Pixel Tablet (2023) x 451 units for xTS Quality Improvement - Devices for automated testing',
        '2 | Resource Groups IN ANDROID_TABLET, CHROME_DESKTOP',
        '1 | Build Est. Date to 1/1/2026',
        '1 | Material Sourcing Est. Date from 1/1/2026',
        '1 | Target Date from 1/1/2025 to 1/1/2026',
      ],
    },
  ];

  testCases.forEach((tc) => {
    it(`${tc.name}`, async () => {
      const queryString = tc.inputUrl.split('?')[1];

      render(
        <FakeContextProvider
          mountedPath="/ui/fleet/requests"
          routerOptions={{
            initialEntries: [`/ui/fleet/requests?${queryString}`],
          }}
        >
          <ShortcutProvider>
            <SettingsProvider>
              <RriPageComponent />
            </SettingsProvider>
          </ShortcutProvider>
        </FakeContextProvider>,
      );

      // Wait for chips to appear and match
      await waitFor(() => {
        const chips = screen.queryAllByTestId('filter-chip');
        const chipTexts = Array.from(chips).map((el) => el.textContent || '');
        expect(chipTexts.sort()).toEqual(tc.expectedChips.sort());
      });
    });
  });
});
