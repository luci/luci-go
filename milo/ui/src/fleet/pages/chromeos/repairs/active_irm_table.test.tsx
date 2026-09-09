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

import { render, screen } from '@testing-library/react';

import { IrmIncident } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ActiveIrmTable } from './active_irm_table';
import * as UseIrmIncidentsModule from './use_irm_incidents';

const MOCK_INCIDENTS: readonly IrmIncident[] = [
  {
    id: '1',
    masterBugId: '  12345678  ',
    title: 'ChromeOS fleet wide lab issue',
  },
  { id: '2', masterBugId: '', title: '' },
];

describe('<ActiveIrmTable />', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  const mockUseIrm = (
    overrides: Partial<
      ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>
    >,
  ) =>
    jest.spyOn(UseIrmIncidentsModule, 'useIrmIncidents').mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: false,
      error: null,
      refetch: jest.fn(),
      ...overrides,
    } as ReturnType<typeof UseIrmIncidentsModule.useIrmIncidents>);

  const renderComponent = () =>
    render(
      <FakeContextProvider>
        <ActiveIrmTable />
      </FakeContextProvider>,
    );

  it('renders loading state', () => {
    mockUseIrm({ isLoading: true });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByRole('progressbar')).toBeInTheDocument();
  });

  it('renders error state', () => {
    mockUseIrm({
      isError: true,
      error: new Error('Failed to load IRM incidents'),
    });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toBeInTheDocument();
    expect(
      screen.getByText(/Failed to load IRM incidents/i),
    ).toBeInTheDocument();
  });

  it('renders empty state', () => {
    mockUseIrm({ data: { irmIncidents: [] } });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByText('No active IRM incidents')).toBeInTheDocument();
  });

  it('renders data and handles edge cases with link and fallbacks', () => {
    mockUseIrm({ data: { irmIncidents: MOCK_INCIDENTS } });
    renderComponent();

    expect(screen.getByText('Active IRM Bugs')).toBeInTheDocument();
    expect(screen.getByText('Bug ID')).toBeInTheDocument();
    expect(screen.getByText('Incident Name')).toBeInTheDocument();

    const link = screen.getByRole('link', { name: /b\/12345678/i });
    expect(link).toHaveAttribute(
      'href',
      'https://b.corp.google.com/issues/12345678',
    );
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer');

    expect(
      screen.getByText('ChromeOS fleet wide lab issue'),
    ).toBeInTheDocument();
    expect(screen.getByText('N/A')).toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });
});
