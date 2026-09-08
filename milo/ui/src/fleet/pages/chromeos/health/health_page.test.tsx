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

import { GetDefaultQuotaResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { HealthPage } from './health_page';
import * as UseDefaultQuotaModule from './use_default_quota';

describe('HealthPage', () => {
  beforeEach(() => {
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
  });

  it('renders overview page with only title, subtitle, and top-right Configuration button', () => {
    render(
      <FakeContextProvider>
        <HealthPage />
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
    // Quota card is NOT on the initial overview dashboard
    expect(
      screen.queryByText('Global Default Expected Quota'),
    ).not.toBeInTheDocument();
  });

  it('navigates to Configuration view showing the quota card and returns on Back', () => {
    render(
      <FakeContextProvider>
        <HealthPage />
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
