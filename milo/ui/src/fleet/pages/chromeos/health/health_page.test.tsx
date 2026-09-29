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
import { fireEvent, render, screen } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import * as UseDeviceDimensionsModule from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import {
  GetDefaultQuotaResponse,
  GetFleetAvailabilityTrendsResponse,
  GetDeviceDimensionsResponse,
  ListSupportRiskIncidentsResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HealthPage } from './health_page';
import * as UseDefaultQuotaModule from './use_default_quota';
import * as UseFleetAvailabilityTrendsModule from './use_fleet_availability_trends';
import * as UseModelQuotaOverridesModule from './use_model_quota_overrides';
import * as UseSupportRiskIncidentsModule from './use_support_risk_incidents';

describe('HealthPage', () => {
  beforeEach(() => {
    jest
      .spyOn(UseDeviceDimensionsModule, 'useDeviceDimensions')
      .mockReturnValue({
        data: {
          baseDimensions: {},
          labels: {
            'label-model': { values: ['volteer', 'brya'] },
            'label-pool': { values: ['DUT_POOL_QUOTA'] },
          },
        } as unknown as GetDeviceDimensionsResponse,
        isPending: false,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDeviceDimensionsResponse, Error>);

    jest
      .spyOn(UseFleetAvailabilityTrendsModule, 'useFleetAvailabilityTrends')
      .mockReturnValue({
        data: { series: [], metricType: 'health' },
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<
        GetFleetAvailabilityTrendsResponse,
        Error
      >);

    jest
      .spyOn(UseModelQuotaOverridesModule, 'useModelQuotaOverrides')
      .mockReturnValue({
        overridesQuery: {
          data: { overrides: [] },
          isPending: false,
          isError: false,
          error: null,
        } as unknown as ReturnType<
          typeof UseModelQuotaOverridesModule.useModelQuotaOverrides
        >['overridesQuery'],
        setOverrideMutation: {
          mutateAsync: jest.fn(),
          isPending: false,
        } as unknown as ReturnType<
          typeof UseModelQuotaOverridesModule.useModelQuotaOverrides
        >['setOverrideMutation'],
        deleteOverrideMutation: {
          mutateAsync: jest.fn(),
          isPending: false,
        } as unknown as ReturnType<
          typeof UseModelQuotaOverridesModule.useModelQuotaOverrides
        >['deleteOverrideMutation'],
        canEdit: true,
        isPermissionLoading: false,
      });

    jest.spyOn(UseDefaultQuotaModule, 'useDefaultQuota').mockReturnValue({
      quotaQuery: {
        data: {
          defaultQuota: 49,
        } as unknown as GetDefaultQuotaResponse,
        isPending: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<GetDefaultQuotaResponse, Error>,
      setQuotaMutation: {
        mutateAsync: jest.fn(),
        isPending: false,
      } as unknown as ReturnType<
        typeof UseDefaultQuotaModule.useDefaultQuota
      >['setQuotaMutation'],
      canEdit: true,
      isPermissionLoading: false,
    });

    jest
      .spyOn(UseSupportRiskIncidentsModule, 'useSupportRiskIncidents')
      .mockReturnValue({
        data: {
          incidents: [
            {
              id: '1',
              model: 'volteer',
              title: 'Battery controller defect',
              buganizerId: 'b/312456789',
            },
          ],
        } as unknown as ListSupportRiskIncidentsResponse,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<ListSupportRiskIncidentsResponse, Error>);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders overview page with header, Configuration button, FilterBar, and Support Risk Incidents panel', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthPage />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('ChromeOS Fleet Health Metrics'),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Analyze hardware reliability trends, monitor model health, and identify pool degradations.',
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('button', { name: /^configuration$/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByPlaceholderText('Add a filter (e.g. model:volteer)'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Active Support Risk Incidents'),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Global Default Expected Quota'),
    ).not.toBeInTheDocument();
  });

  it('applies model filter when Show button on Support Risk incident is clicked', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthPage />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    const showButton = screen.getByRole('button', { name: /^show$/i });
    expect(showButton).toBeInTheDocument();
    fireEvent.click(showButton);
  });

  it('navigates to Configuration view showing the quota card and returns on Back', () => {
    render(
      <FakeContextProvider>
        <ShortcutProvider>
          <HealthPage />
        </ShortcutProvider>
      </FakeContextProvider>,
    );

    const configButton = screen.getByRole('button', {
      name: /^configuration$/i,
    });
    fireEvent.click(configButton);

    expect(screen.getByText('Fleet Health Configuration')).toBeInTheDocument();
    expect(
      screen.getByText('Global Default Expected Quota'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('Manual Model Quota Overrides'),
    ).toBeInTheDocument();

    const backButton = screen.getByRole('button', {
      name: /back to fleet overview/i,
    });
    fireEvent.click(backButton);

    expect(
      screen.getByText('ChromeOS Fleet Health Metrics'),
    ).toBeInTheDocument();
    expect(
      screen.queryByText('Global Default Expected Quota'),
    ).not.toBeInTheDocument();
  });
});
