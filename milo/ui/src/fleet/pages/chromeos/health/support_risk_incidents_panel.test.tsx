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

import {
  ListSupportRiskIncidentsResponse,
  SupportRiskIncident,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { SupportRiskIncidentsPanel } from './support_risk_incidents_panel';
import * as UseSupportRiskIncidentsModule from './use_support_risk_incidents';

describe('SupportRiskIncidentsPanel', () => {
  const mockIncidents: SupportRiskIncident[] = [
    {
      id: '1',
      model: 'brya',
      buganizerId: '1001',
      title: 'Brya trackpad failure batch',
    },
    {
      id: '2',
      model: 'brask',
      buganizerId: '1002',
      title: 'Brask USB controller glitch',
    },
  ];

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('renders active incidents list with model names and issue titles', () => {
    jest
      .spyOn(UseSupportRiskIncidentsModule, 'useSupportRiskIncidents')
      .mockReturnValue({
        data: { incidents: mockIncidents } as ListSupportRiskIncidentsResponse,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<ListSupportRiskIncidentsResponse, Error>);

    const onShowModel = jest.fn();

    render(
      <FakeContextProvider>
        <SupportRiskIncidentsPanel onShowModel={onShowModel} />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Active Support Risk Incidents'),
    ).toBeInTheDocument();

    expect(
      screen.getByText('b/1001 - Brya trackpad failure batch'),
    ).toBeInTheDocument();
    expect(screen.getByText('brya')).toBeInTheDocument();

    expect(
      screen.getByText('b/1002 - Brask USB controller glitch'),
    ).toBeInTheDocument();
    expect(screen.getByText('brask')).toBeInTheDocument();

    const showButtons = screen.getAllByRole('button', { name: /^show$/i });
    expect(showButtons).toHaveLength(2);

    fireEvent.click(showButtons[0]);
    expect(onShowModel).toHaveBeenCalledWith('brya');
  });

  it('renders clean empty state when there are no active incidents', () => {
    jest
      .spyOn(UseSupportRiskIncidentsModule, 'useSupportRiskIncidents')
      .mockReturnValue({
        data: { incidents: [] } as ListSupportRiskIncidentsResponse,
        isLoading: false,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<ListSupportRiskIncidentsResponse, Error>);

    render(
      <FakeContextProvider>
        <SupportRiskIncidentsPanel />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Active Support Risk Incidents'),
    ).toBeInTheDocument();
    expect(
      screen.getByText('No active Support Risk incidents.'),
    ).toBeInTheDocument();
  });

  it('renders loading state when query is pending', () => {
    jest
      .spyOn(UseSupportRiskIncidentsModule, 'useSupportRiskIncidents')
      .mockReturnValue({
        data: undefined,
        isLoading: true,
        isError: false,
        error: null,
      } as unknown as UseQueryResult<ListSupportRiskIncidentsResponse, Error>);

    render(
      <FakeContextProvider>
        <SupportRiskIncidentsPanel />
      </FakeContextProvider>,
    );

    expect(screen.getByLabelText('Loading incidents')).toBeInTheDocument();
  });

  it('renders error alert when query fails', () => {
    jest
      .spyOn(UseSupportRiskIncidentsModule, 'useSupportRiskIncidents')
      .mockReturnValue({
        data: undefined,
        isLoading: false,
        isError: true,
        error: new Error('Network error'),
      } as unknown as UseQueryResult<ListSupportRiskIncidentsResponse, Error>);

    render(
      <FakeContextProvider>
        <SupportRiskIncidentsPanel />
      </FakeContextProvider>,
    );

    expect(
      screen.getByText('Failed to load support risk incidents.'),
    ).toBeInTheDocument();
  });
});
